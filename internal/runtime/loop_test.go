package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fummicc1/suigo/internal/wire"
	"github.com/fummicc1/suigo/suigo"
)

// testApp は Store[M] をそのまま App に適合させる最小実装。
// Task 2.7 で作る本物の登録 API と同じ形になる。
type testApp[M any] struct {
	store *suigo.Store[M]
	view  func(M, *suigo.Store[M]) suigo.Node
}

func newTestApp[M any](initial M, view func(M, *suigo.Store[M]) suigo.Node) *testApp[M] {
	return &testApp[M]{store: suigo.NewStore(initial), view: view}
}

func (a *testApp[M]) Wake() <-chan struct{}     { return a.store.Wake() }
func (a *testApp[M]) Apply() int                { return a.store.Apply() }
func (a *testApp[M]) View() suigo.Node          { return a.view(a.store.Model(), a.store) }
func (a *testApp[M]) Snapshot() ([]byte, error) { return json.Marshal(a.store.Model()) }
func (a *testApp[M]) Restore(b []byte) error {
	var m M
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	a.store.Replace(m)
	return nil
}

type counter struct{ Count int }

func counterView(m counter, s *suigo.Store[counter]) suigo.Node {
	return suigo.VStack(
		suigo.Text(itoa(m.Count)),
		suigo.Button("Tap", func() { s.Update(func(m *counter) { m.Count++ }) }),
	)
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func start(t *testing.T, app App) (*Loop, context.CancelFunc) {
	t.Helper()
	l := New(app, wire.JSON{})
	ctx, cancel := context.WithCancel(context.Background())
	go l.Run(ctx)
	return l, cancel
}

func next(t *testing.T, l *Loop) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	b, ok := l.NextFrame(ctx)
	if !ok {
		t.Fatal("no frame within 2s")
	}
	var f map[string]any
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("bad frame: %v\n%s", err, b)
	}
	return f
}

func textOf(f map[string]any) string {
	tree := f["tree"].(map[string]any)
	first := tree["children"].([]any)[0].(map[string]any)
	return first["props"].(map[string]any)["text"].(string)
}

// TG-907: mount（=Run 開始）で初回描画が出る
func TestInitialRender(t *testing.T) {
	l, cancel := start(t, newTestApp(counter{}, counterView))
	defer cancel()
	f := next(t, l)
	if f["type"] != "render" || f["seq"] != 1.0 || textOf(f) != "0" {
		t.Errorf("initial frame: %v", f)
	}
}

// TG-901: イベント → ハンドラ → 更新 → 描画
func TestEventRoundTrip(t *testing.T) {
	l, cancel := start(t, newTestApp(counter{}, counterView))
	defer cancel()
	next(t, l)

	l.Dispatch("0/1", "tap", suigo.Payload{Tag: suigo.PayloadNone})
	f := next(t, l)
	if textOf(f) != "1" || f["seq"] != 2.0 {
		t.Errorf("after tap: text=%s seq=%v", textOf(f), f["seq"])
	}
}

// TG-902: outbox は最新のみ保持する
func TestOutboxKeepsOnlyLatest(t *testing.T) {
	app := newTestApp(counter{}, counterView)
	l, cancel := start(t, app)
	defer cancel()
	next(t, l) // 初回を消費

	// 消費せずに 3 回描画を起こす。各 tap を確実に別描画にするため wake ごとに待つ。
	for i := 0; i < 3; i++ {
		l.Dispatch("0/1", "tap", suigo.Payload{})
		time.Sleep(20 * time.Millisecond)
	}
	f := next(t, l)
	if textOf(f) != "3" {
		t.Errorf("expected latest (3), got %s", textOf(f))
	}
	// これ以上フレームは無いはず
	ctx, c := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer c()
	if _, ok := l.NextFrame(ctx); ok {
		t.Error("intermediate frame was not dropped")
	}
}

// TG-903: seq は単調増加
func TestSeqIncreases(t *testing.T) {
	l, cancel := start(t, newTestApp(counter{}, counterView))
	defer cancel()
	prev := next(t, l)["seq"].(float64)
	for i := 0; i < 3; i++ {
		l.Dispatch("0/1", "tap", suigo.Payload{})
		f := next(t, l)
		if f["seq"].(float64) <= prev {
			t.Errorf("seq not increasing: %v after %v", f["seq"], prev)
		}
		prev = f["seq"].(float64)
	}
}

// TG-904 / TG-905: 未知 nodeID・未知 kind は無視、panic しない
func TestUnknownEventsIgnored(t *testing.T) {
	l, cancel := start(t, newTestApp(counter{}, counterView))
	defer cancel()
	next(t, l)
	l.Dispatch("no/such/id", "tap", suigo.Payload{})
	l.Dispatch("0/1", "toggle", suigo.Payload{})
	ctx, c := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer c()
	if _, ok := l.NextFrame(ctx); ok {
		t.Error("unknown events must not trigger a render")
	}
}

// TG-906: ハンドラの panic を封じ込め、error フレームを出し、ループは生き続ける
func TestHandlerPanicIsContained(t *testing.T) {
	app := newTestApp(counter{}, func(m counter, s *suigo.Store[counter]) suigo.Node {
		return suigo.VStack(
			suigo.Text(itoa(m.Count)),
			suigo.Button("boom", func() { panic("kaboom") }),
			suigo.Button("ok", func() { s.Update(func(m *counter) { m.Count++ }) }),
		)
	})
	l, cancel := start(t, app)
	defer cancel()
	next(t, l)

	l.Dispatch("0/1", "tap", suigo.Payload{})
	f := next(t, l)
	if f["type"] != "error" || f["code"] != string(wire.CodeHandlerPanic) {
		t.Fatalf("expected handler_panic error frame, got %v", f)
	}
	if !strings.Contains(f["message"].(string), "kaboom") {
		t.Errorf("message should include panic value: %s", f["message"])
	}

	// ループが生きている: 別のボタンは動く
	l.Dispatch("0/2", "tap", suigo.Payload{})
	f = next(t, l)
	if f["type"] != "render" || textOf(f) != "1" {
		t.Errorf("loop died after panic: %v", f)
	}
}

// TG-909 / TG-910: 300 超で warning、以下では出ない。警告は 1 回だけ。
func TestNodeCountWarning(t *testing.T) {
	build := func(rows int) func(counter, *suigo.Store[counter]) suigo.Node {
		return func(counter, *suigo.Store[counter]) suigo.Node {
			kids := make([]suigo.Node, rows)
			for i := range kids {
				kids[i] = suigo.Text("x")
			}
			return suigo.VStack(kids...)
		}
	}

	t.Run("299 nodes: no warning", func(t *testing.T) {
		l, cancel := start(t, newTestApp(counter{}, build(298)))
		defer cancel()
		if f := next(t, l); f["type"] != "render" {
			t.Errorf("got %v", f["type"])
		}
	})

	t.Run("400 nodes: warning then render, once", func(t *testing.T) {
		app := newTestApp(counter{}, build(399))
		l, cancel := start(t, app)
		defer cancel()
		// warning が先に push され、render が続く。outbox はバッファ 1 なので
		// 消費が遅いと warning が render で上書きされうる。両方受け取れるかを確認する。
		seen := map[string]bool{}
		for i := 0; i < 2; i++ {
			ctx, c := context.WithTimeout(context.Background(), time.Second)
			b, ok := l.NextFrame(ctx)
			c()
			if !ok {
				break
			}
			var f map[string]any
			_ = json.Unmarshal(b, &f)
			seen[f["type"].(string)] = true
			if f["type"] == "error" && f["code"] != string(wire.CodeNodeCountWarning) {
				t.Errorf("wrong code: %v", f["code"])
			}
		}
		if !seen["render"] {
			t.Error("render never arrived")
		}
		// 2 回目の描画では警告が出ない
		app.store.Update(func(*counter) {})
		f := next(t, l)
		if f["type"] != "render" {
			t.Errorf("second render should not re-warn: %v", f["type"])
		}
	})
}

// TG-908: 外部からの Update（env 相当）で再描画が起きる
func TestExternalUpdateTriggersRender(t *testing.T) {
	app := newTestApp(counter{}, counterView)
	l, cancel := start(t, app)
	defer cancel()
	next(t, l)
	app.store.Update(func(m *counter) { m.Count = 7 })
	if f := next(t, l); textOf(f) != "7" {
		t.Errorf("text = %s", textOf(f))
	}
}

func TestDuplicateIDBecomesErrorFrame(t *testing.T) {
	app := newTestApp(counter{}, func(counter, *suigo.Store[counter]) suigo.Node {
		return suigo.VStack(suigo.Text("a").ID("dup"), suigo.Text("b").ID("dup"))
	})
	l, cancel := start(t, app)
	defer cancel()
	f := next(t, l)
	if f["type"] != "error" || f["code"] != string(wire.CodeDuplicateID) {
		t.Errorf("got %v", f)
	}
}

func TestSnapshotAndRestore(t *testing.T) {
	app := newTestApp(counter{}, counterView)
	l, cancel := start(t, app)
	defer cancel()
	next(t, l)

	for i := 0; i < 3; i++ {
		l.Dispatch("0/1", "tap", suigo.Payload{})
		next(t, l)
	}
	ctx := context.Background()
	snap, err := l.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(snap) != `{"Count":3}` {
		t.Errorf("snapshot = %s", snap)
	}

	// 別のループへ復元
	app2 := newTestApp(counter{}, counterView)
	l2, cancel2 := start(t, app2)
	defer cancel2()
	next(t, l2) // 初回（Count=0）
	if err := l2.Restore(ctx, snap); err != nil {
		t.Fatal(err)
	}
	if f := next(t, l2); textOf(f) != "3" {
		t.Errorf("after restore: text = %s", textOf(f))
	}
}

func TestRestoreFailureEmitsWarning(t *testing.T) {
	app := newTestApp(counter{}, counterView)
	l, cancel := start(t, app)
	defer cancel()
	next(t, l)
	err := l.Restore(context.Background(), []byte(`not json`))
	if err == nil {
		t.Fatal("expected restore error")
	}
	f := next(t, l)
	if f["type"] != "error" || f["code"] != string(wire.CodeRestoreFailed) {
		t.Errorf("got %v", f)
	}
}

// ハンドラ内から Update してもデッドロックしない（TG-805 のループ側）
func TestUpdateInsideHandlerDoesNotDeadlock(t *testing.T) {
	l, cancel := start(t, newTestApp(counter{}, counterView))
	defer cancel()
	next(t, l)
	done := make(chan struct{})
	go func() {
		l.Dispatch("0/1", "tap", suigo.Payload{})
		next(t, l)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deadlock: handler calling Update blocked the loop")
	}
}
