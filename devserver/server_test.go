//go:build suigodev

package devserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/fummicc1/suigo/internal/wire"
	"github.com/fummicc1/suigo/suigo"
)

type counter struct{ Count int }

type testApp struct{ store *suigo.Store[counter] }

func (a *testApp) Wake() <-chan struct{} { return a.store.Wake() }
func (a *testApp) Apply() int            { return a.store.Apply() }
func (a *testApp) View() suigo.Node {
	m := a.store.Model()
	return suigo.VStack(
		suigo.Text(itoa(m.Count)),
		suigo.Button("Tap", func() { a.store.Update(func(m *counter) { m.Count++ }) }),
	)
}
func (a *testApp) Snapshot() ([]byte, error) { return json.Marshal(a.store.Model()) }
func (a *testApp) Restore(b []byte) error {
	var m counter
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	a.store.Replace(m)
	return nil
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

// host は serve に対するホスト役。stdin へ書き、stdout から読む。
type host struct {
	toServer io.WriteCloser
	fromServ io.Reader
	t        *testing.T
}

func newHost(t *testing.T, app suigo.App) (*host, context.CancelFunc) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = serve(ctx, app, inR, outW); _ = outW.Close() }()
	return &host{toServer: inW, fromServ: outR, t: t}, cancel
}

func (h *host) send(frame any) {
	h.t.Helper()
	b, _ := (wire.JSON{}).Encode(frame)
	if err := wire.WriteFrame(h.toServer, b); err != nil {
		h.t.Fatal(err)
	}
}

func (h *host) recv() map[string]any {
	h.t.Helper()
	type res struct {
		f   map[string]any
		err error
	}
	ch := make(chan res, 1)
	go func() {
		b, err := wire.ReadFrame(h.fromServ)
		if err != nil {
			ch <- res{nil, err}
			return
		}
		var f map[string]any
		ch <- res{f, json.Unmarshal(b, &f)}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			h.t.Fatalf("recv: %v", r.err)
		}
		return r.f
	case <-time.After(2 * time.Second):
		h.t.Fatal("recv: timeout")
		return nil
	}
}

func textOf(f map[string]any) string {
	return f["tree"].(map[string]any)["children"].([]any)[0].(map[string]any)["props"].(map[string]any)["text"].(string)
}

func TestMountThenRender(t *testing.T) {
	h, cancel := newHost(t, &testApp{store: suigo.NewStore(counter{})})
	defer cancel()
	h.send(wire.MountFrame{Type: "mount"})
	f := h.recv()
	if f["type"] != "render" || textOf(f) != "0" {
		t.Errorf("first frame: %v", f)
	}
}

func TestNoRenderBeforeMount(t *testing.T) {
	inR, inW := io.Pipe()
	var out bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = serve(ctx, &testApp{store: suigo.NewStore(counter{})}, inR, &out); close(done) }()
	time.Sleep(100 * time.Millisecond)
	if out.Len() != 0 {
		t.Errorf("rendered before mount: %d bytes", out.Len())
	}
	cancel()
	_ = inW.Close()
	<-done
}

func TestEventRoundTripOverStdio(t *testing.T) {
	h, cancel := newHost(t, &testApp{store: suigo.NewStore(counter{})})
	defer cancel()
	h.send(wire.MountFrame{Type: "mount"})
	h.recv()
	h.send(wire.EventFrame{Type: "event", NodeID: "0/1", Kind: "tap", Payload: wire.WirePayload{T: suigo.PayloadNone}})
	if f := h.recv(); textOf(f) != "1" {
		t.Errorf("after tap: %s", textOf(f))
	}
}

func TestRestoreOnMount(t *testing.T) {
	h, cancel := newHost(t, &testApp{store: suigo.NewStore(counter{})})
	defer cancel()
	h.send(wire.MountFrame{Type: "mount", Restore: []byte(`{"Count":7}`)})
	// 初回 render（Count=0）→ restore 後の render（Count=7）。順序は outbox の最新優先により
	// どちらか一方だけ届く可能性があるので、7 が届くまで読む。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f := h.recv(); f["type"] == "render" && textOf(f) == "7" {
			return
		}
	}
	t.Fatal("restored value never rendered")
}

func TestSnapshotRequest(t *testing.T) {
	h, cancel := newHost(t, &testApp{store: suigo.NewStore(counter{Count: 3})})
	defer cancel()
	h.send(wire.MountFrame{Type: "mount"})
	h.recv()
	h.send(wire.SnapshotFrame{Type: "snapshot"})
	f := h.recv()
	if f["type"] != "snapshot_result" {
		t.Fatalf("got %v", f)
	}
	if m := f["model"].(map[string]any); m["Count"] != 3.0 {
		t.Errorf("model = %v", m)
	}
}

func TestUnknownFrameIgnored(t *testing.T) {
	h, cancel := newHost(t, &testApp{store: suigo.NewStore(counter{})})
	defer cancel()
	h.send(wire.MountFrame{Type: "mount"})
	h.recv()
	h.send(map[string]any{"type": "telepathy"})
	h.send(wire.EventFrame{Type: "event", NodeID: "0/1", Kind: "tap", Payload: wire.WirePayload{T: suigo.PayloadNone}})
	if f := h.recv(); textOf(f) != "1" {
		t.Errorf("server should survive unknown frame: %v", f)
	}
}

func TestStdinCloseStopsServer(t *testing.T) {
	inR, inW := io.Pipe()
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, &testApp{store: suigo.NewStore(counter{})}, inR, io.Discard) }()
	_ = inW.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop when stdin closed")
	}
}
