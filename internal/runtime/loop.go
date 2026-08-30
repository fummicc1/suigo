// Package runtime はレンダーループを実装する。ストアの変更を適用し、View を評価し、
// 採番して符号化し、outbox へ出す。イベントを受けてハンドラを呼ぶのもここである。
//
// すべてが単一の goroutine で順に起きる。イベント処理 → 変更の適用 → 描画 という順序が
// 保たれるため、ハンドラの中から Store.Update を呼んでも再入しない。
package runtime

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/fummicc1/suigo/internal/wire"
	"github.com/fummicc1/suigo/suigo"
)

// NodeCountWarnThreshold を超えるツリーは warning を出す。docs/wire-format.md §7。
// PRE-3 の実測で、全ノードが変わる更新は 511 ノードで予算超過だった。
const NodeCountWarnThreshold = 300

// App は suigo.App の別名。Loop はジェネリクスを消去したこの姿だけを見る。
type App = suigo.App

type event struct {
	nodeID  string
	kind    string
	payload suigo.Payload
}

// Loop は 1 つのアプリのレンダーループ。
type Loop struct {
	app      App
	enc      wire.Encoder
	events   chan event
	requests chan request
	outbox   chan []byte
	handlers suigo.Handlers
	seq      uint64
	warned   bool
}

type request struct {
	kind  string
	body  []byte
	reply chan []byte
}

// New はループを作る。Run を呼ぶまで何も動かない。
func New(app App, enc wire.Encoder) *Loop {
	return &Loop{
		app:      app,
		enc:      enc,
		events:   make(chan event, 256),
		requests: make(chan request, 8),
		// バッファ 1・最新優先。詰まっているときは古いフレームを捨てて置き換える（FR-4.5）。
		outbox: make(chan []byte, 1),
	}
}

// Dispatch はイベントをループへ渡す。任意の goroutine から呼べ、すぐに戻る。
func (l *Loop) Dispatch(nodeID, kind string, p suigo.Payload) {
	select {
	case l.events <- event{nodeID, kind, p}:
	default:
		// 256 件溜まっているのは異常。捨てるが、ログの手段はまだ無いので黙って落とす。
	}
}

// NextFrame は次のフレームが出るまで待つ。ctx が終わると ok=false。
func (l *Loop) NextFrame(ctx context.Context) ([]byte, bool) {
	select {
	case b := <-l.outbox:
		return b, true
	case <-ctx.Done():
		return nil, false
	}
}

// Snapshot はループの goroutine にモデルの取り出しを依頼し、結果を待つ。
func (l *Loop) Snapshot(ctx context.Context) ([]byte, error) {
	return l.ask(ctx, "snapshot", nil)
}

// Restore はループの goroutine にモデルの復元を依頼する。
func (l *Loop) Restore(ctx context.Context, model []byte) error {
	_, err := l.ask(ctx, "restore", model)
	return err
}

func (l *Loop) ask(ctx context.Context, kind string, body []byte) ([]byte, error) {
	req := request{kind: kind, body: body, reply: make(chan []byte, 1)}
	select {
	case l.requests <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case r := <-req.reply:
		if len(r) > 0 && r[0] == 0 {
			return nil, fmt.Errorf("%s", r[1:])
		}
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run はループ本体。呼び出し元の goroutine を占有し、ctx が終わるまで戻らない。
// 最初に 1 回描画する。
func (l *Loop) Run(ctx context.Context) {
	l.render()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-l.events:
			l.handle(ev)
			// ハンドラが Update を積んでいれば wake が来る。ここでは描画しない。
		case <-l.app.Wake():
			l.render()
		case req := <-l.requests:
			l.serve(req)
		}
	}
}

func (l *Loop) handle(ev event) {
	byKind, ok := l.handlers[ev.nodeID]
	if !ok {
		return
	}
	h, ok := byKind[ev.kind]
	if !ok {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			l.emitError(wire.SeverityRuntime, wire.CodeHandlerPanic,
				fmt.Sprintf("handler for %s %s panicked: %v\n%s", ev.nodeID, ev.kind, r, debug.Stack()))
		}
	}()
	h(ev.payload)
}

func (l *Loop) render() {
	l.app.Apply()

	tree, err := l.safeView()
	if err != nil {
		l.emitError(wire.SeverityRuntime, wire.CodeHandlerPanic, err.Error())
		return
	}

	assigned, err := suigo.Assign(tree)
	if err != nil {
		l.emitAssignError(err)
		return
	}
	l.handlers = assigned.Handlers

	if assigned.Count > NodeCountWarnThreshold && !l.warned {
		l.warned = true
		l.emitError(wire.SeverityWarning, wire.CodeNodeCountWarning,
			fmt.Sprintf("render tree has %d nodes (practical limit is about %d)", assigned.Count, NodeCountWarnThreshold))
	}

	l.seq++
	b, err := l.enc.Encode(wire.RenderFrame{Type: "render", Seq: l.seq, Tree: wire.FromAssigned(assigned.Root)})
	if err != nil {
		l.emitError(wire.SeverityRuntime, wire.CodeHandlerPanic, "encode: "+err.Error())
		return
	}
	l.push(b)
}

func (l *Loop) safeView() (tree suigo.Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("View panicked: %v\n%s", r, debug.Stack())
		}
	}()
	return l.app.View(), nil
}

func (l *Loop) emitAssignError(err error) {
	code := wire.CodeDuplicateID
	switch {
	case isErr(err, suigo.ErrInvalidID):
		code = wire.CodeInvalidID
	case isErr(err, suigo.ErrTooDeep):
		code = wire.CodeInvalidID
	}
	l.emitError(wire.SeverityRuntime, code, err.Error())
}

func (l *Loop) emitError(sev wire.Severity, code wire.ErrorCode, msg string) {
	b, err := l.enc.Encode(wire.ErrorFrame{Type: "error", Severity: sev, Code: code, Message: msg})
	if err != nil {
		return
	}
	l.push(b)
}

// push は outbox へ入れる。詰まっていれば古いものを捨てて最新で置き換える。
func (l *Loop) push(b []byte) {
	select {
	case l.outbox <- b:
		return
	default:
	}
	select {
	case <-l.outbox:
	default:
	}
	select {
	case l.outbox <- b:
	default:
	}
}

func (l *Loop) serve(req request) {
	switch req.kind {
	case "snapshot":
		b, err := l.app.Snapshot()
		if err != nil {
			req.reply <- append([]byte{0}, err.Error()...)
			return
		}
		req.reply <- b
	case "restore":
		if err := l.app.Restore(req.body); err != nil {
			l.emitError(wire.SeverityWarning, wire.CodeRestoreFailed,
				"restore failed, falling back to zero value: "+err.Error())
			req.reply <- append([]byte{0}, err.Error()...)
			return
		}
		req.reply <- []byte{}
		l.render()
	default:
		req.reply <- append([]byte{0}, "unknown request "+req.kind...)
	}
}

func isErr(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
