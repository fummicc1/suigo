// Package wire はホストと Go の間を流れるフレームを定義し、符号化・復号する。
// 契約は docs/wire-format.md にあり、この実装はそれに従属する。
package wire

import "github.com/fummicc1/suigo/suigo"

// Severity は error フレームの重大度。
type Severity string

const (
	SeverityBuild   Severity = "build"
	SeverityRuntime Severity = "runtime"
	SeverityWarning Severity = "warning"
)

// ErrorCode は機械可読なエラー識別子。テストはメッセージ本文ではなくこれで判定する。
type ErrorCode string

const (
	CodeBuildFailed      ErrorCode = "build_failed"
	CodeDuplicateID      ErrorCode = "duplicate_id"
	CodeInvalidID        ErrorCode = "invalid_id"
	CodeHandlerPanic     ErrorCode = "handler_panic"
	CodeNodeCountWarning ErrorCode = "node_count_warning"
	CodeRestoreFailed    ErrorCode = "restore_failed"
)

// ---- Go → ホスト ----

type RenderFrame struct {
	Type string    `json:"type"`
	Seq  uint64    `json:"seq"`
	Tree WireNode  `json:"tree"`
}

type ErrorFrame struct {
	Type     string    `json:"type"`
	Severity Severity  `json:"severity"`
	Code     ErrorCode `json:"code"`
	Message  string    `json:"message"`
}

type SnapshotResultFrame struct {
	Type  string `json:"type"`
	Model any    `json:"model,omitempty"`
	Error string `json:"error,omitempty"`
}

// ---- ホスト → Go ----

type MountFrame struct {
	Type    string `json:"type"`
	Restore []byte `json:"restore,omitempty"`
	// StartSeq は最初の render に使う seq の 1 つ手前。dev の proxy が再起動のたびに設定する。
	// seq はプロセスではなく**接続**の通し番号である。子は再起動で 1 から振り直すため、
	// これを渡さないとホストが「古いフレーム」として捨ててしまう（FR-4.5）。
	StartSeq uint64 `json:"startSeq,omitempty"`
}

type EventFrame struct {
	Type    string      `json:"type"`
	NodeID  string      `json:"nodeID"`
	Kind    string      `json:"kind"`
	Payload WirePayload `json:"payload"`
}

type EnvFrame struct {
	Type        string    `json:"type"`
	ColorScheme string    `json:"colorScheme"`
	SafeArea    SafeArea  `json:"safeArea"`
	SizeClass   SizeClass `json:"sizeClass"`
}

type SafeArea struct {
	Top      float64 `json:"top"`
	Bottom   float64 `json:"bottom"`
	Leading  float64 `json:"leading"`
	Trailing float64 `json:"trailing"`
}

type SizeClass struct {
	H string `json:"h"`
	V string `json:"v"`
}

type SnapshotFrame struct {
	Type string `json:"type"`
}

// WireNode は採番済みノードのワイヤ表現。空のフィールドは省略する。
type WireNode struct {
	ID       string           `json:"id"`
	Type     suigo.NodeType   `json:"type"`
	Props    map[string]any   `json:"props,omitempty"`
	Mods     []WireModifier   `json:"mods,omitempty"`
	Children []WireNode       `json:"children,omitempty"`
}

type WireModifier struct {
	K string `json:"k"`
	V any    `json:"v"`
}

// WirePayload は閉じたタグ付き共用体。任意の JSON は許さない。
type WirePayload struct {
	T suigo.PayloadTag `json:"t"`
	V any              `json:"v,omitempty"`
}

// FromAssigned は採番済みツリーをワイヤ表現へ変換する。
func FromAssigned(n suigo.AssignedNode) WireNode {
	out := WireNode{ID: n.ID, Type: n.Type, Props: n.Props}
	if len(n.Mods) > 0 {
		out.Mods = make([]WireModifier, len(n.Mods))
		for i, m := range n.Mods {
			out.Mods[i] = WireModifier{K: m.K, V: m.V}
		}
	}
	if len(n.Children) > 0 {
		out.Children = make([]WireNode, len(n.Children))
		for i, c := range n.Children {
			out.Children[i] = FromAssigned(c)
		}
	}
	return out
}

// ToPayload はワイヤ表現から Go 側の Payload へ変換する。
// タグと値の型が食い違う場合はゼロ値にする（panic しない）。
func ToPayload(p WirePayload) suigo.Payload {
	out := suigo.Payload{Tag: p.T}
	switch p.T {
	case suigo.PayloadString:
		out.String, _ = p.V.(string)
	case suigo.PayloadBool:
		out.Bool, _ = p.V.(bool)
	case suigo.PayloadInt:
		out.Int = toInt(p.V)
	case suigo.PayloadDouble:
		out.Double, _ = p.V.(float64)
	case suigo.PayloadIndex:
		out.Index = toInt(p.V)
	}
	return out
}

// FromPayload は Go 側の Payload をワイヤ表現へ変換する。
func FromPayload(p suigo.Payload) WirePayload {
	switch p.Tag {
	case suigo.PayloadString:
		return WirePayload{T: p.Tag, V: p.String}
	case suigo.PayloadBool:
		return WirePayload{T: p.Tag, V: p.Bool}
	case suigo.PayloadInt:
		return WirePayload{T: p.Tag, V: p.Int}
	case suigo.PayloadDouble:
		return WirePayload{T: p.Tag, V: p.Double}
	case suigo.PayloadIndex:
		return WirePayload{T: p.Tag, V: p.Index}
	default:
		return WirePayload{T: suigo.PayloadNone}
	}
}

// encoding/json は数値を float64 に復号するため、int を取り出すときは変換が要る。
func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	}
	return 0
}
