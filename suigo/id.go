package suigo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MaxDepth はツリーの深さ上限。docs/wire-format.md §7。
const MaxDepth = 256

// ErrInvalidID は明示 ID に "/" が含まれるときに返る。
var ErrInvalidID = errors.New("invalid node ID")

// ErrDuplicateID は同一ツリー内で ID が重複したときに返る。
var ErrDuplicateID = errors.New("duplicate node ID")

// ErrTooDeep はツリーが MaxDepth を超えたときに返る。
var ErrTooDeep = errors.New("tree too deep")

// Handlers は採番済みツリーのイベント経路。nodeID → kind → Handler。
type Handlers map[string]map[string]Handler

// Assigned は採番済みのツリー。ワイヤに出す ID と、Go 側に残るハンドラを分けて持つ。
type Assigned struct {
	Root     AssignedNode
	Handlers Handlers
	Count    int
}

// AssignedNode は ID が確定したノード。ハンドラは持たない（ワイヤに出す形）。
type AssignedNode struct {
	ID       string
	Type     NodeType
	Props    map[string]any
	Mods     []Modifier
	Children []AssignedNode
}

// Assign はツリー全体に ID を振り、ハンドラを集約する。
//
// 既定の ID は構造パス（ルートが "0"、i 番目の子が "<親>/i"）。
// 明示 ID があればそれを使い、子孫の接頭辞にもなる。
func Assign(root Node) (Assigned, error) {
	a := &assigner{
		seen:     make(map[string]int),
		handlers: make(Handlers),
	}
	out, err := a.visit(root, "0", 0)
	if err != nil {
		return Assigned{}, err
	}
	for id, c := range a.seen {
		if c > 1 {
			return Assigned{}, fmt.Errorf("%w: %q (%d occurrences)", ErrDuplicateID, id, c)
		}
	}
	return Assigned{Root: out, Handlers: a.handlers, Count: a.count}, nil
}

type assigner struct {
	seen     map[string]int
	handlers Handlers
	count    int
}

func (a *assigner) visit(n Node, structural string, depth int) (AssignedNode, error) {
	if depth >= MaxDepth {
		return AssignedNode{}, fmt.Errorf("%w: exceeds %d at %q", ErrTooDeep, MaxDepth, structural)
	}
	id := structural
	if n.explicitID != "" {
		if strings.Contains(n.explicitID, "/") {
			return AssignedNode{}, fmt.Errorf("%w: explicit ID must not contain \"/\": got %q", ErrInvalidID, n.explicitID)
		}
		id = n.explicitID
	}
	a.seen[id]++
	a.count++

	if len(n.handlers) > 0 {
		byKind := make(map[string]Handler, len(n.handlers))
		for _, h := range n.handlers {
			byKind[h.kind] = h.fn
		}
		a.handlers[id] = byKind
	}

	out := AssignedNode{ID: id, Type: n.Type, Props: n.Props, Mods: n.Mods}
	if len(n.Children) > 0 {
		out.Children = make([]AssignedNode, 0, len(n.Children))
		for i, c := range n.Children {
			child, err := a.visit(c, id+"/"+strconv.Itoa(i), depth+1)
			if err != nil {
				return AssignedNode{}, err
			}
			out.Children = append(out.Children, child)
		}
	}
	return out, nil
}
