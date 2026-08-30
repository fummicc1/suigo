package suigo

// NodeType はワイヤ上の "type" フィールドに対応する。
// 値は docs/wire-format.md §3.2 と一致していなければならない。
type NodeType string

const (
	TypeText         NodeType = "Text"
	TypeButton       NodeType = "Button"
	TypeVStack       NodeType = "VStack"
	TypeHStack       NodeType = "HStack"
	TypeZStack       NodeType = "ZStack"
	TypeSpacer       NodeType = "Spacer"
	TypeDivider      NodeType = "Divider"
	TypeScrollView   NodeType = "ScrollView"
	TypeList         NodeType = "List"
	TypeForEach      NodeType = "ForEach"
	TypeTextField    NodeType = "TextField"
	TypeToggle       NodeType = "Toggle"
	TypeProgressView NodeType = "ProgressView"
	TypeImage        NodeType = "Image"
)

// Modifier は SwiftUI の修飾子 1 つに対応する。適用順に意味があるため、
// Node は Modifier をマップではなくスライスで保持する。
type Modifier struct {
	K string
	V any
}

// Payload はイベントに添えられる値。ワイヤ上ではタグ付き共用体になる。
type Payload struct {
	Tag    PayloadTag
	String string
	Bool   bool
	Int    int
	Double float64
	Index  int
}

type PayloadTag string

const (
	PayloadNone   PayloadTag = "none"
	PayloadString PayloadTag = "string"
	PayloadBool   PayloadTag = "bool"
	PayloadInt    PayloadTag = "int"
	PayloadDouble PayloadTag = "double"
	PayloadIndex  PayloadTag = "index"
)

// Handler はイベントに応答するクロージャ。ワイヤには出ない。
type Handler func(Payload)

type handlerEntry struct {
	kind string
	fn   Handler
}

// Node は UI ツリーの 1 要素。値として扱い、修飾子メソッドは常に新しい Node を返す。
//
// handlers をマップではなくスライスにしているのは、Node をコピーしたときに
// 共有されないようにするため。マップだとコピー同士が同じ実体を指し、不変性が壊れる。
type Node struct {
	explicitID string
	Type       NodeType
	Props      map[string]any
	Mods       []Modifier
	Children   []Node
	handlers   []handlerEntry
}

// ID は明示的な識別子を与える。ForEach の行では必須。
// "/" を含む値は採番時に invalid_id エラーになる（docs/wire-format.md §6）。
func (n Node) ID(id string) Node {
	n.explicitID = id
	return n
}

func (n Node) withMod(k string, v any) Node {
	mods := make([]Modifier, len(n.Mods), len(n.Mods)+1)
	copy(mods, n.Mods)
	n.Mods = append(mods, Modifier{K: k, V: v})
	return n
}

func (n Node) withProp(k string, v any) Node {
	props := make(map[string]any, len(n.Props)+1)
	for pk, pv := range n.Props {
		props[pk] = pv
	}
	props[k] = v
	n.Props = props
	return n
}

func (n Node) withHandler(kind string, fn Handler) Node {
	if fn == nil {
		return n
	}
	hs := make([]handlerEntry, len(n.handlers), len(n.handlers)+1)
	copy(hs, n.handlers)
	n.handlers = append(hs, handlerEntry{kind: kind, fn: fn})
	return n
}

func newNode(t NodeType, children ...Node) Node {
	n := Node{Type: t}
	if len(children) > 0 {
		n.Children = append([]Node(nil), children...)
	}
	return n
}
