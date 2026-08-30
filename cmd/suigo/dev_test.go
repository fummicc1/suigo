package main

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/fummicc1/suigo/internal/wire"
)

// ホットリロードの本丸を、実際の proxy と子プロセスの再起動を通して検証する。
//
// このテストが無かったために本物のバグを取り逃がした: 子は再起動のたびに seq を 1 から
// 振り直すが、ホストから見た接続は 1 本なので、ホストは番号が戻ったフレームを
// 「古い」として捨てる（FR-4.5）。結果、Go 側は正しくリロードされているのに画面が変わらない。
// 手動の検証スクリプトは seq を無視して読んでいたため、すり抜けていた。

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

type probeHost struct {
	conn net.Conn
	t    *testing.T
}

func (h *probeHost) recvRender(timeout time.Duration) map[string]any {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_ = h.conn.SetReadDeadline(time.Now().Add(time.Until(deadline)))
		b, err := wire.ReadFrame(h.conn)
		if err != nil {
			h.t.Fatalf("read frame: %v", err)
		}
		var f map[string]any
		if err := json.Unmarshal(b, &f); err != nil {
			continue
		}
		if f["type"] == "render" {
			return f
		}
	}
	h.t.Fatal("no render frame within timeout")
	return nil
}

func (h *probeHost) send(frame any) {
	h.t.Helper()
	b, _ := (wire.JSON{}).Encode(frame)
	if err := wire.WriteFrame(h.conn, b); err != nil {
		h.t.Fatal(err)
	}
}

func startDevServer(t *testing.T) (*devServer, *probeHost, context.CancelFunc) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a child process")
	}
	port := freePort(t)
	d := &devServer{port: port, pkg: "./examples/counter/dev", watchDir: "../.."}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = d.run(ctx) }()

	var conn net.Conn
	var err error
	deadline := time.Now().Add(60 * time.Second) // 初回ビルドは冷えていると十数秒かかる
	for time.Now().Before(deadline) {
		conn, err = net.DialTimeout("tcp", d.addr(), time.Second)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	h := &probeHost{conn: conn, t: t}
	h.send(wire.MountFrame{Type: "mount"})
	return d, h, func() { conn.Close(); cancel() }
}

// リロードをまたいで seq が単調増加する。ホストの FR-4.5 の規則で捨てられてはならない。
func TestSeqIsMonotonicAcrossReload(t *testing.T) {
	d, h, stop := startDevServer(t)
	defer stop()

	first := h.recvRender(30 * time.Second)
	lastSeq := uint64(first["seq"].(float64))
	if lastSeq == 0 {
		t.Fatalf("first seq = %v, want >= 1", first["seq"])
	}
	t.Logf("initial render seq = %d", lastSeq)

	// ソースは変えずに再起動だけ起こす。子の seq は 1 に戻るが、proxy が続きを渡すはず。
	for i := 0; i < 3; i++ {
		d.reload(context.Background(), "test")
		f := h.recvRender(30 * time.Second)
		seq := uint64(f["seq"].(float64))
		t.Logf("after reload %d: seq = %d", i+1, seq)
		if seq <= lastSeq {
			t.Fatalf("reload %d: seq went from %d to %d; a host applying the FR-4.5 rule would drop this frame and the screen would never update",
				i+1, lastSeq, seq)
		}
		lastSeq = seq
	}
}
