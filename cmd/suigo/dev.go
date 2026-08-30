package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fummicc1/suigo/internal/wire"
)

// devServer は supervisor 兼 proxy。
//
//	ホスト ──TCP(固定ポート、切れない)──> devServer ──stdio──> Go 子プロセス(再起動される)
//
// ホストは子プロセスに直接つながない。子が再起動しても TCP は維持され、画面がちらつかない（design §0-4）。
// v0.1 の M1 段階では「1 回ビルドして起動し、中継する」までを実装し、監視・再起動は Task 5 で足す。
type devServer struct {
	port     int
	pkg      string
	watchDir string
	metrics  bool

	mu            sync.Mutex
	host          net.Conn        // 現在のホスト。後勝ち（design §4.4）
	child         *child          // 現在の子プロセス
	lastFrame     []byte          // 最新の render。新規接続へ即送る（design §4.4）
	snapshotReply chan<- []byte   // reload 中だけ非 nil。子からの snapshot_result を受け取る
	reloads       []time.Duration // --metrics 用
	generation    int             // 起動用バイナリの世代。実行中のファイルを上書きしないため
}

type child struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan struct{}
}

func (d *devServer) run(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", d.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	defer ln.Close()
	log.Printf("suigo dev: listening on %s", addr)

	if err := d.restart(ctx); err != nil {
		return err
	}

	go func() {
		if err := d.watch(ctx); err != nil {
			log.Printf("suigo dev: watcher stopped: %v", err)
		}
	}()
	go func() { <-ctx.Done(); ln.Close() }()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				d.stopChild()
				if d.metrics {
					d.printMetrics()
				}
				return nil
			}
			return err
		}
		d.attachHost(conn)
	}
}

// attachHost は新しいホストを受け入れる。前のホストは切る（後勝ち）。
// 接続直後に最新の render を送り、次の状態変化を待たずに描画できるようにする。
func (d *devServer) attachHost(conn net.Conn) {
	d.mu.Lock()
	if d.host != nil {
		log.Printf("suigo dev: replacing host %s with %s", d.host.RemoteAddr(), conn.RemoteAddr())
		d.host.Close()
	}
	d.host = conn
	last := d.lastFrame
	d.mu.Unlock()

	log.Printf("suigo dev: host connected from %s", conn.RemoteAddr())
	if last != nil {
		_ = wire.WriteFrame(conn, last)
	}
	go d.pumpHostToChild(conn)
}

// pumpHostToChild はホストからのフレームを子の stdin へ流す。
func (d *devServer) pumpHostToChild(conn net.Conn) {
	defer func() {
		d.mu.Lock()
		if d.host == conn {
			d.host = nil
		}
		d.mu.Unlock()
		conn.Close()
		log.Printf("suigo dev: host disconnected %s", conn.RemoteAddr())
	}()
	for {
		b, err := wire.ReadFrame(conn)
		if err != nil {
			return
		}
		d.mu.Lock()
		c := d.child
		d.mu.Unlock()
		if c == nil {
			continue
		}
		if err := wire.WriteFrame(c.stdin, b); err != nil {
			log.Printf("suigo dev: write to child: %v", err)
		}
	}
}

// pumpChildToHost は子の stdout からのフレームをホストへ流し、最新の render を保持する。
func (d *devServer) pumpChildToHost(c *child, stdout io.Reader) {
	defer close(c.done)
	for {
		b, err := wire.ReadFrame(stdout)
		if err != nil {
			return
		}
		typ, _ := wire.PeekType(wire.JSON{}, b)
		switch typ {
		case "render":
			d.mu.Lock()
			d.lastFrame = b
			d.mu.Unlock()
		case "snapshot_result":
			// reload 中の snapshot 応答は proxy が消費し、ホストへは流さない。
			var f wire.SnapshotResultFrame
			d.mu.Lock()
			reply := d.snapshotReply
			d.mu.Unlock()
			if reply != nil {
				if err := (wire.JSON{}).Decode(b, &f); err == nil && f.Error == "" {
					model, _ := (wire.JSON{}).Encode(f.Model)
					reply <- model
				} else {
					reply <- nil
				}
			}
			continue
		}
		d.mu.Lock()
		h := d.host
		d.mu.Unlock()
		if h != nil {
			_ = wire.WriteFrame(h, b)
		}
	}
}

// restart は子プロセスをビルドして起動し、mount を送る。
func (d *devServer) restart(ctx context.Context) error {
	bin, err := d.build(ctx)
	if err != nil {
		return err
	}
	return d.start(ctx, bin, nil)
}

// build は固定の出力パスへビルドし、起動用に別名へコピーして返す。
// 出力先を毎回変えると go build のリンク結果キャッシュが効かず、温ビルドでも約 0.4s 遅くなる。
// 実行中の旧バイナリを上書きしないよう、起動用は世代ごとに別ファイルにする。
func (d *devServer) build(ctx context.Context) (string, error) {
	stable := filepath.Join(os.TempDir(), "suigo-dev-"+sanitize(d.pkg))
	t0 := time.Now()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "go", "build", "-tags", "suigodev", "-o", stable, d.pkg)
	cmd.Dir = d.watchDir
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build %s: %w\n%s", d.pkg, err, strings.TrimSpace(stderr.String()))
	}
	d.generation++
	out := fmt.Sprintf("%s.%d", stable, d.generation)
	if err := copyFile(stable, out); err != nil {
		return "", err
	}
	log.Printf("suigo dev: built %s in %s", d.pkg, time.Since(t0).Round(time.Millisecond))
	return out, nil
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "_", ".", "_", " ", "_").Replace(s)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (d *devServer) start(ctx context.Context, bin string, restore []byte) error {
	cmd := exec.CommandContext(ctx, bin)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start child: %w", err)
	}
	c := &child{cmd: cmd, stdin: stdin, done: make(chan struct{})}

	d.mu.Lock()
	d.child = c
	d.mu.Unlock()

	go d.pumpChildToHost(c, stdout)

	mount, _ := (wire.JSON{}).Encode(wire.MountFrame{Type: "mount", Restore: restore})
	if err := wire.WriteFrame(stdin, mount); err != nil {
		return fmt.Errorf("send mount: %w", err)
	}
	log.Printf("suigo dev: child started (pid %d)", cmd.Process.Pid)
	return nil
}

func (d *devServer) stopChild() {
	d.mu.Lock()
	c := d.child
	d.child = nil
	d.mu.Unlock()
	if c == nil {
		return
	}
	c.stdin.Close()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
}
