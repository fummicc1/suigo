package suigo

import (
	"reflect"
	"testing"
)

// TG-001
func TestVStackKeepsChildOrder(t *testing.T) {
	n := VStack(Text("a"), Text("b"))
	if len(n.Children) != 2 {
		t.Fatalf("children = %d, want 2", len(n.Children))
	}
	if n.Children[0].Props["text"] != "a" || n.Children[1].Props["text"] != "b" {
		t.Errorf("children out of order: %v", n.Children)
	}
}

// TG-002
func TestModifierChainOrder(t *testing.T) {
	n := Text("a").Font(Title).Padding(8)
	got := []string{n.Mods[0].K, n.Mods[1].K}
	want := []string{"font", "padding"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mods = %v, want %v", got, want)
	}
}

// TG-003: padding→background と background→padding は別物でなければならない。
// SwiftUI では適用順で描画結果が変わる。
func TestModifierOrderIsPreserved(t *testing.T) {
	a := Text("a").Padding(8).Background(Red)
	b := Text("a").Background(Red).Padding(8)
	if reflect.DeepEqual(a.Mods, b.Mods) {
		t.Fatal("modifier order was lost; padding→background must differ from background→padding")
	}
	if a.Mods[0].K != "padding" || b.Mods[0].K != "background" {
		t.Errorf("first modifiers: a=%s b=%s", a.Mods[0].K, b.Mods[0].K)
	}
}

// TG-004: 修飾子は新しい Node を返す。元は変わらない。
func TestNodeIsImmutable(t *testing.T) {
	n := Text("a")
	n2 := n.Padding(8)
	if len(n.Mods) != 0 {
		t.Errorf("original mutated: mods = %v", n.Mods)
	}
	if len(n2.Mods) != 1 {
		t.Errorf("derived: mods = %v, want 1", n2.Mods)
	}

	// props も共有されない
	n3 := n.withProp("text", "changed")
	if n.Props["text"] != "a" {
		t.Errorf("original props mutated: %v", n.Props)
	}
	if n3.Props["text"] != "changed" {
		t.Errorf("derived props: %v", n3.Props)
	}

	// handlers も共有されない
	h := Button("x", func() {})
	h2 := h.withHandler("extra", func(Payload) {})
	if len(h.handlers) != 1 || len(h2.handlers) != 2 {
		t.Errorf("handlers shared: original=%d derived=%d", len(h.handlers), len(h2.handlers))
	}
}

// TG-005: 14 種すべてが正しい Type で構築できる
func TestAllPrimitivesConstruct(t *testing.T) {
	cases := map[NodeType]Node{
		TypeText:         Text("t"),
		TypeButton:       Button("b", nil),
		TypeVStack:       VStack(),
		TypeHStack:       HStack(),
		TypeZStack:       ZStack(),
		TypeSpacer:       Spacer(),
		TypeDivider:      Divider(),
		TypeScrollView:   ScrollView(),
		TypeList:         List(),
		TypeForEach:      ForEach([]int{1}, func(i int) string { return "x" }, func(int) Node { return Text("") }),
		TypeTextField:    TextField("l", "v", nil),
		TypeToggle:       Toggle("l", true, nil),
		TypeProgressView: ProgressView(0.5),
		TypeImage:        Symbol("star"),
	}
	if len(cases) != 14 {
		t.Fatalf("expected 14 primitives, got %d", len(cases))
	}
	for want, n := range cases {
		if n.Type != want {
			t.Errorf("Type = %q, want %q", n.Type, want)
		}
	}
}

func TestTextFieldDefaults(t *testing.T) {
	n := TextField("label", "value", nil)
	if n.Props["commit"] != "submit" {
		t.Errorf("default commit = %v, want submit", n.Props["commit"])
	}
	n = n.CommitOn(CommitDebounce(300))
	if n.Props["commit"] != "debounce:300" {
		t.Errorf("commit = %v, want debounce:300", n.Props["commit"])
	}
}

func TestHandlersAreCapturedNotSerialized(t *testing.T) {
	called := false
	n := Button("tap", func() { called = true })
	if len(n.handlers) != 1 || n.handlers[0].kind != "tap" {
		t.Fatalf("handler not registered: %+v", n.handlers)
	}
	n.handlers[0].fn(Payload{})
	if !called {
		t.Error("handler did not invoke closure")
	}
	if _, leaked := n.Props["onTap"]; leaked {
		t.Error("handler leaked into props")
	}
}

func TestNilHandlerIsIgnored(t *testing.T) {
	if len(Button("b", nil).handlers) != 0 {
		t.Error("nil handler should not be registered")
	}
}

func TestFrameOptionsOmitZeroAndMapInfinity(t *testing.T) {
	n := Text("").Frame(FrameOptions{MaxWidth: Infinity, Height: 44})
	got := n.Mods[0].V.(map[string]any)
	if got["maxWidth"] != "infinity" {
		t.Errorf("maxWidth = %v, want \"infinity\"", got["maxWidth"])
	}
	if got["height"] != 44.0 {
		t.Errorf("height = %v, want 44", got["height"])
	}
	if _, has := got["width"]; has {
		t.Error("zero width should be omitted")
	}
}

func TestEdgeInsetsOmitZero(t *testing.T) {
	got := Text("").PaddingEdges(EdgeInsets{Top: 8, Leading: 4}).Mods[0].V.(map[string]any)
	if len(got) != 2 || got["top"] != 8.0 || got["leading"] != 4.0 {
		t.Errorf("insets = %v", got)
	}
}
