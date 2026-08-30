package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/fummicc1/suigo/internal/wire"
)

const (
	debounce        = 100 * time.Millisecond
	snapshotTimeout = 200 * time.Millisecond
	killTimeout     = 2 * time.Second
)

// watch は Go ソースの変更を監視し、デバウンス後に reload を呼ぶ。ctx が終わるまで戻らない。
func (d *devServer) watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	// fsnotify は再帰監視しないため、ディレクトリを歩いて全部登録する。
	if err := filepath.WalkDir(d.watchDir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !e.IsDir() {
			return nil
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") && path != d.watchDir || name == "Sources" || name == "Tests" || name == "testdata" {
			return filepath.SkipDir
		}
		return w.Add(path)
	}); err != nil {
		return err
	}
	log.Printf("suigo dev: watching %s for *.go changes", d.watchDir)

	var timer *time.Timer
	var pending []string
	fire := make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-w.Events:
			if !strings.HasSuffix(ev.Name, ".go") || strings.HasSuffix(ev.Name, "_test.go") {
				continue
			}
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			pending = append(pending, filepath.Base(ev.Name))
			if timer == nil {
				timer = time.AfterFunc(debounce, func() {
					select {
					case fire <- struct{}{}:
					default:
					}
				})
			} else {
				timer.Reset(debounce)
			}
		case err := <-w.Errors:
			log.Printf("suigo dev: watch error: %v", err)
		case <-fire:
			timer = nil
			sort.Strings(pending)
			changed := strings.Join(uniq(pending), ", ")
			pending = nil
			d.reload(ctx, changed)
		}
	}
}

// reload は design §2.2 の手順 2〜8 を実行する。
//
//  1. 旧プロセスは動かしたままビルドする
//  2. 失敗なら error(build) をホストへ送り、旧プロセスを維持する（FR-6.5）
//  3. 成功なら旧プロセスに snapshot を要求する（200ms でタイムアウト）
//  4. 旧プロセスを止め、新プロセスを restore つきで起動する
//  5. 復元失敗は子が warning を出す（FR-6.4）
func (d *devServer) reload(ctx context.Context, changed string) {
	t0 := time.Now()
	log.Printf("suigo dev: change detected (%s); rebuilding", changed)

	bin, err := d.build(ctx)
	if err != nil {
		d.sendToHost(wire.ErrorFrame{
			Type: "error", Severity: wire.SeverityBuild, Code: wire.CodeBuildFailed,
			Message: err.Error(),
		})
		log.Printf("suigo dev: build failed; keeping previous process running")
		return
	}
	tBuild := time.Since(t0)

	snap := d.snapshot(ctx)
	tSnap := time.Since(t0) - tBuild

	d.stopChild()
	if err := d.start(ctx, bin, snap); err != nil {
		log.Printf("suigo dev: restart failed: %v", err)
		return
	}
	total := time.Since(t0)
	d.recordReload(total)
	log.Printf("suigo dev: reloaded in %s (build %s, snapshot %s, restart %s)",
		total.Round(time.Millisecond), tBuild.Round(time.Millisecond),
		tSnap.Round(time.Millisecond), (total - tBuild - tSnap).Round(time.Millisecond))
}

// snapshot は現在の子に snapshot を要求し、snapshot_result の model を返す。
// 子が応答しない・失敗した場合は nil を返し、新プロセスはゼロ値から始まる。
func (d *devServer) snapshot(ctx context.Context) []byte {
	d.mu.Lock()
	c := d.child
	d.mu.Unlock()
	if c == nil {
		return nil
	}

	reply := make(chan []byte, 1)
	d.mu.Lock()
	d.snapshotReply = reply
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.snapshotReply = nil
		d.mu.Unlock()
	}()

	req, _ := (wire.JSON{}).Encode(wire.SnapshotFrame{Type: "snapshot"})
	if err := wire.WriteFrame(c.stdin, req); err != nil {
		return nil
	}
	select {
	case model := <-reply:
		return model
	case <-time.After(snapshotTimeout):
		log.Printf("suigo dev: snapshot timed out after %s; restarting with zero value", snapshotTimeout)
		return nil
	case <-ctx.Done():
		return nil
	}
}

func (d *devServer) sendToHost(frame any) {
	b, err := (wire.JSON{}).Encode(frame)
	if err != nil {
		return
	}
	d.mu.Lock()
	h := d.host
	d.mu.Unlock()
	if h != nil {
		_ = wire.WriteFrame(h, b)
	}
}

func (d *devServer) recordReload(dur time.Duration) {
	d.mu.Lock()
	d.reloads = append(d.reloads, dur)
	d.mu.Unlock()
}

// printMetrics は --metrics 指定時、終了時に p50 / p90 を出す（NFR-1 / AC-4 の測定手段）。
func (d *devServer) printMetrics() {
	d.mu.Lock()
	rs := append([]time.Duration(nil), d.reloads...)
	d.mu.Unlock()
	if len(rs) == 0 {
		fmt.Fprintln(os.Stderr, "suigo dev: no reloads recorded")
		return
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
	p := func(q float64) time.Duration { return rs[int(float64(len(rs)-1)*q)] }
	fmt.Fprintf(os.Stderr, "suigo dev: %d reloads — p50 %s, p90 %s, max %s (NFR-1 target: p50 < 1.5s)\n",
		len(rs), p(0.5).Round(time.Millisecond), p(0.9).Round(time.Millisecond), rs[len(rs)-1].Round(time.Millisecond))
}

func uniq(ss []string) []string {
	out := ss[:0]
	for i, s := range ss {
		if i == 0 || s != ss[i-1] {
			out = append(out, s)
		}
	}
	return out
}
