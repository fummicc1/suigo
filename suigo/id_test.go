package suigo

import (
	"errors"
	"strings"
	"testing"
)

func collectIDs(n AssignedNode, acc *[]string) {
	*acc = append(*acc, n.ID)
	for _, c := range n.Children {
		collectIDs(c, acc)
	}
}

func ids(t *testing.T, n Node) []string {
	t.Helper()
	a, err := Assign(n)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	var out []string
	collectIDs(a.Root, &out)
	return out
}

// TG-201
func TestStructuralPathIDs(t *testing.T) {
	got := ids(t, VStack(Text("a"), HStack(Text("b"), Text("c"))))
	want := []string{"0", "0/0", "0/1", "0/1/0", "0/1/1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

// TG-202
func TestExplicitIDWins(t *testing.T) {
	got := ids(t, Text("a").ID("hello"))
	if got[0] != "hello" {
		t.Errorf("id = %q, want hello", got[0])
	}
}

// TG-203
func TestExplicitIDPrefixesDescendants(t *testing.T) {
	got := ids(t, VStack(Text("a"), Text("b")).ID("row-7"))
	want := []string{"row-7", "row-7/0", "row-7/1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

// TG-204: 同じツリーを 2 回採番しても ID は一致する
func TestIDsAreStableAcrossRenders(t *testing.T) {
	build := func() Node {
		return VStack(Text("a"), ForEach([]string{"x", "y"},
			func(s string) string { return s },
			func(s string) Node { return Text(s) }))
	}
	a := ids(t, build())
	b := ids(t, build())
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Errorf("unstable: %v vs %v", a, b)
	}
}

// TG-205: 1 か所だけ変えても他の ID は変わらない。PRE-3 の性能特性の前提。
func TestOneChangeDoesNotShiftOtherIDs(t *testing.T) {
	before := ids(t, VStack(Text("a"), Text("b"), Text("c")))
	after := ids(t, VStack(Text("a"), Text("b"), Text("CHANGED")))
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("ids shifted: %v -> %v", before, after)
	}
}

// ForEach をワイヤに残すことで、後続の兄弟の ID が行数に依存しない（wire-format §6.1）
func TestForEachDoesNotShiftSiblingIDs(t *testing.T) {
	build := func(n int) Node {
		items := make([]int, n)
		for i := range items {
			items[i] = i
		}
		return VStack(
			ForEach(items, func(i int) string { return "item-" + itoa(i) }, func(int) Node { return Text("") }),
			Text("footer"),
		)
	}
	three := ids(t, build(3))
	four := ids(t, build(4))
	if three[len(three)-1] != "0/1" || four[len(four)-1] != "0/1" {
		t.Errorf("footer id changed with row count: 3 rows -> %s, 4 rows -> %s",
			three[len(three)-1], four[len(four)-1])
	}
}

// TG-206
func TestHandlersCollectedByID(t *testing.T) {
	tapped := false
	a, err := Assign(VStack(Text("a"), Button("tap", func() { tapped = true })))
	if err != nil {
		t.Fatal(err)
	}
	h := a.Handlers["0/1"]["tap"]
	if h == nil {
		t.Fatalf("handler missing: %v", a.Handlers)
	}
	h(Payload{})
	if !tapped {
		t.Error("handler did not fire")
	}
	if _, has := a.Handlers["0/0"]; has {
		t.Error("Text should have no handlers")
	}
}

// TG-301
func TestDuplicateIDDetected(t *testing.T) {
	_, err := Assign(VStack(Text("a").ID("dup"), Text("b").ID("dup")))
	if !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("err = %v, want ErrDuplicateID", err)
	}
	if !strings.Contains(err.Error(), `"dup"`) || !strings.Contains(err.Error(), "2 occurrences") {
		t.Errorf("message should name the ID and count: %v", err)
	}
}

// TG-302
func TestEmptyExplicitIDFallsBack(t *testing.T) {
	got := ids(t, Text("a").ID(""))
	if got[0] != "0" {
		t.Errorf("id = %q, want structural 0", got[0])
	}
}

// TG-303
func TestSlashInExplicitIDRejected(t *testing.T) {
	_, err := Assign(Text("a").ID("a/b"))
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("err = %v, want ErrInvalidID", err)
	}
	if !strings.Contains(err.Error(), `"a/b"`) {
		t.Errorf("message should name the ID: %v", err)
	}
}

// TG-304: "/" を拒否することで、明示 ID と構造パスの衝突が起きない
func TestSlashRejectionPreventsCollision(t *testing.T) {
	// .ID("x") の子は "x/0" になる。もし .ID("x/0") が許されると衝突する。
	_, err := Assign(VStack(VStack(Text("a")).ID("x"), Text("b").ID("x/0")))
	if !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected the explicit \"x/0\" to be rejected, got %v", err)
	}
}

// TG-401
func TestEmptyTree(t *testing.T) {
	a, err := Assign(VStack())
	if err != nil {
		t.Fatal(err)
	}
	if a.Root.ID != "0" || a.Count != 1 {
		t.Errorf("root=%q count=%d", a.Root.ID, a.Count)
	}
}

func deep(n int) Node {
	node := Text("bottom")
	for i := 0; i < n-1; i++ {
		node = VStack(node)
	}
	return node
}

// TG-402
func TestDeepTreeWithinLimit(t *testing.T) {
	a, err := Assign(deep(100))
	if err != nil {
		t.Fatal(err)
	}
	if a.Count != 100 {
		t.Errorf("count = %d, want 100", a.Count)
	}
}

func TestDepthAtLimitAccepted(t *testing.T) {
	if _, err := Assign(deep(MaxDepth)); err != nil {
		t.Errorf("depth %d should be accepted: %v", MaxDepth, err)
	}
}

func TestDepthOverLimitRejected(t *testing.T) {
	_, err := Assign(deep(MaxDepth + 1))
	if !errors.Is(err, ErrTooDeep) {
		t.Errorf("depth %d should be rejected, got %v", MaxDepth+1, err)
	}
}

// TG-403
func TestLargeTree(t *testing.T) {
	kids := make([]Node, 333)
	for i := range kids {
		kids[i] = HStack(Text("a"), Text("b"))
	}
	a, err := Assign(VStack(kids...))
	if err != nil {
		t.Fatal(err)
	}
	if a.Count != 1+333*3 {
		t.Errorf("count = %d", a.Count)
	}
}

// TG-211
func TestForEachAppliesIDFunc(t *testing.T) {
	type item struct{ id, name string }
	items := []item{{"t1", "milk"}, {"t2", "book"}}
	got := ids(t, ForEach(items, func(i item) string { return i.id }, func(i item) Node { return Text(i.name) }))
	want := []string{"0", "t1", "t2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

// TG-212
func TestForEachEmpty(t *testing.T) {
	n := ForEach([]int{}, func(int) string { return "" }, func(int) Node { return Text("") })
	if n.Type != TypeForEach || len(n.Children) != 0 {
		t.Errorf("type=%s children=%d", n.Type, len(n.Children))
	}
}

func BenchmarkAssign511(b *testing.B) {
	kids := make([]Node, 170)
	for i := range kids {
		kids[i] = HStack(Text("item"), Button("done", func() {}))
	}
	tree := VStack(kids...)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Assign(tree); err != nil {
			b.Fatal(err)
		}
	}
}
