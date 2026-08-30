package wire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fummicc1/suigo/suigo"
)

const goldenDir = "../../testdata/wire/golden"

func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(goldenDir, name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return b
}

// golden との比較は「復号した構造の一致」で行う（FR-3.7）。
// バイト列比較だとキー順やインデントで壊れ、Encoder を差し替えたときにも壊れる。
func assertSameStructure(t *testing.T, got, want []byte) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("unmarshal got: %v\n%s", err, got)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		gs, _ := json.MarshalIndent(g, "", "  ")
		ws, _ := json.MarshalIndent(w, "", "  ")
		t.Errorf("structure mismatch\n--- got ---\n%s\n--- want ---\n%s", gs, ws)
	}
}

func encodeTree(t *testing.T, enc Encoder, seq uint64, root suigo.Node) []byte {
	t.Helper()
	a, err := suigo.Assign(root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := enc.Encode(RenderFrame{Type: "render", Seq: seq, Tree: FromAssigned(a.Root)})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TG-501 (counter)
func TestGoldenCounter(t *testing.T) {
	tree := suigo.VStack(
		suigo.Text("0").Font(suigo.LargeTitle).ForegroundColor(suigo.Blue),
		suigo.Button("Tap", func() {}).Padding(8),
	).Spacing(12)
	assertSameStructure(t, encodeTree(t, JSON{}, 1, tree), readGolden(t, "render_counter.json"))
}

// TG-501 (todo3): ForEach と明示 ID を含む
func TestGoldenTodo3(t *testing.T) {
	type todo struct {
		id, label string
		done      bool
	}
	items := []todo{{"todo-1", "牛乳を買う", true}, {"todo-2", "本を返す", false}, {"todo-3", "散歩する", false}}
	tree := suigo.VStack(
		suigo.HStack(
			suigo.TextField("新しいタスク", "", nil),
			suigo.Button("追加", nil),
		),
		suigo.Divider(),
		suigo.List(
			suigo.ForEach(items, func(i todo) string { return i.id }, func(i todo) suigo.Node {
				return suigo.HStack(
					suigo.Toggle(i.label, i.done, nil),
					suigo.Spacer(),
					suigo.Button("削除", nil),
				)
			}),
		),
		suigo.Text("3 件").Font(suigo.Caption),
	).Spacing(8)
	assertSameStructure(t, encodeTree(t, JSON{}, 7, tree), readGolden(t, "render_todo3.json"))
}

// TG-501 (all primitives)
func TestGoldenAllPrimitives(t *testing.T) {
	tree := suigo.VStack(
		suigo.Text("text"),
		suigo.Button("button", nil),
		suigo.HStack(suigo.Text("in hstack")).Alignment(suigo.AlignTop),
		suigo.ZStack(suigo.Text("in zstack")).Alignment(suigo.AlignTopLeading),
		suigo.Spacer(),
		suigo.Divider(),
		suigo.ScrollView(suigo.Text("in scroll")),
		suigo.List(suigo.ForEach([]string{"a", "b"},
			func(s string) string { return "row-" + s },
			func(s string) suigo.Node { return suigo.Text("row " + s) })),
		suigo.TextField("field", "abc", nil).CommitOn(suigo.CommitDebounce(300)),
		suigo.Toggle("toggle", true, nil),
		suigo.ProgressView(0.42),
		suigo.Symbol("star.fill"),
		suigo.Asset("AppLogo"),
		suigo.IndeterminateProgress(),
	)
	assertSameStructure(t, encodeTree(t, JSON{}, 1, tree), readGolden(t, "render_all_primitives.json"))
}

// TG-501 (all modifiers) + TG-503: 順序込みで一致
func TestGoldenAllModifiers(t *testing.T) {
	tree := suigo.VStack(
		suigo.Text("padding then background").Padding(8).Background(suigo.Gray),
		suigo.Text("background then padding").Background(suigo.Gray).Padding(8),
		suigo.Text("all").
			PaddingEdges(suigo.EdgeInsets{Top: 8, Bottom: 4, Leading: 12, Trailing: 12}).
			Font(suigo.Headline).
			ForegroundColor(suigo.Hex("#FF8800")).
			Background(suigo.Clear).
			Frame(suigo.FrameOptions{Width: 200, Height: 44, MaxWidth: suigo.Infinity, Alignment: suigo.AlignLeading}).
			CornerRadius(12).
			Disabled(true).
			Hidden(false),
	).Spacing(10)
	assertSameStructure(t, encodeTree(t, JSON{}, 1, tree), readGolden(t, "render_all_modifiers.json"))
}

// TG-501 (deep 100)
func TestGoldenDeep100(t *testing.T) {
	node := suigo.Text("bottom")
	for i := 0; i < 99; i++ {
		node = suigo.VStack(node)
	}
	assertSameStructure(t, encodeTree(t, JSON{}, 1, node), readGolden(t, "render_deep_100.json"))
}

// TG-505: 6 種のペイロードすべてが往復する
func TestGoldenEventsRoundTrip(t *testing.T) {
	var file struct {
		Frames []json.RawMessage `json:"frames"`
	}
	if err := json.Unmarshal(readGolden(t, "events.json"), &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Frames) != 6 {
		t.Fatalf("expected 6 event fixtures, got %d", len(file.Frames))
	}
	enc := JSON{}
	for _, raw := range file.Frames {
		var ev EventFrame
		if err := enc.Decode(raw, &ev); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// wire → Go → wire で構造が保たれる
		back := EventFrame{Type: "event", NodeID: ev.NodeID, Kind: ev.Kind, Payload: FromPayload(ToPayload(ev.Payload))}
		got, err := enc.Encode(back)
		if err != nil {
			t.Fatal(err)
		}
		assertSameStructure(t, got, raw)
	}
}

func TestGoldenEnv(t *testing.T) {
	var env EnvFrame
	if err := (JSON{}).Decode(readGolden(t, "env.json"), &env); err != nil {
		t.Fatal(err)
	}
	if env.ColorScheme != "dark" || env.SafeArea.Top != 59 || env.SafeArea.Bottom != 34 ||
		env.SizeClass.H != "compact" || env.SizeClass.V != "regular" {
		t.Errorf("env decoded wrong: %+v", env)
	}
	got, _ := (JSON{}).Encode(env)
	assertSameStructure(t, got, readGolden(t, "env.json"))
}

func TestGoldenErrorCodes(t *testing.T) {
	var file struct {
		Frames []ErrorFrame `json:"frames"`
	}
	if err := json.Unmarshal(readGolden(t, "errors.json"), &file); err != nil {
		t.Fatal(err)
	}
	want := map[ErrorCode]Severity{
		CodeBuildFailed: SeverityBuild, CodeDuplicateID: SeverityRuntime, CodeInvalidID: SeverityRuntime,
		CodeHandlerPanic: SeverityRuntime, CodeNodeCountWarning: SeverityWarning, CodeRestoreFailed: SeverityWarning,
	}
	if len(file.Frames) != len(want) {
		t.Fatalf("fixture has %d codes, wire.go defines %d", len(file.Frames), len(want))
	}
	for _, f := range file.Frames {
		if sev, ok := want[f.Code]; !ok {
			t.Errorf("fixture code %q is not defined in wire.go", f.Code)
		} else if sev != f.Severity {
			t.Errorf("code %q: fixture severity %q, wire.go %q", f.Code, f.Severity, sev)
		}
	}
}

// TG-502: props はタグ付きでない生の値
func TestPropsAreRawValues(t *testing.T) {
	b := encodeTree(t, JSON{}, 1, suigo.Text("Hello"))
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	props := raw["tree"].(map[string]any)["props"].(map[string]any)
	if props["text"] != "Hello" {
		t.Errorf(`props.text = %v, want raw "Hello"`, props["text"])
	}
}

// TG-504: ハンドラはワイヤに出ない
func TestHandlersNotSerialized(t *testing.T) {
	b := encodeTree(t, JSON{}, 1, suigo.Button("x", func() {}))
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	tree := raw["tree"].(map[string]any)
	for k := range tree {
		if k == "handlers" || k == "onTap" {
			t.Errorf("handler leaked into wire: key %q", k)
		}
	}
	if len(tree) != 3 { // id, type, props
		t.Errorf("unexpected keys: %v", tree)
	}
}

// TG-507: 空の props / mods / children はキーごと省略
func TestEmptyFieldsOmitted(t *testing.T) {
	b := encodeTree(t, JSON{}, 1, suigo.Spacer())
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	tree := raw["tree"].(map[string]any)
	for _, k := range []string{"props", "mods", "children"} {
		if _, has := tree[k]; has {
			t.Errorf("empty %q should be omitted", k)
		}
	}
}

// TG-506: Encoder を差し替えても golden テストの仕組みは通る（形式非依存の証明）。
// ここでは JSON を別の JSON 実装に見立てて差し替える。
type indentedJSON struct{}

func (indentedJSON) Encode(f any) ([]byte, error)     { return json.MarshalIndent(f, "", "\t") }
func (indentedJSON) Decode(b []byte, into any) error   { return json.Unmarshal(b, into) }

func TestEncoderIsSwappable(t *testing.T) {
	tree := suigo.VStack(suigo.Text("0").Font(suigo.LargeTitle).ForegroundColor(suigo.Blue),
		suigo.Button("Tap", nil).Padding(8)).Spacing(12)
	assertSameStructure(t, encodeTree(t, indentedJSON{}, 1, tree), readGolden(t, "render_counter.json"))
}

// TG-601: エンコード不能な値は panic ではなくエラー
func TestUnencodableValueReturnsError(t *testing.T) {
	_, err := (JSON{}).Encode(WireNode{ID: "0", Type: "Text", Props: map[string]any{"fn": func() {}}})
	if err == nil {
		t.Fatal("expected error for func value")
	}
}

// TG-702: 深さ 1000 でも panic しない（Assign が上限で止める）
func TestVeryDeepTreeDoesNotPanic(t *testing.T) {
	node := suigo.Text("")
	for i := 0; i < 1000; i++ {
		node = suigo.VStack(node)
	}
	if _, err := suigo.Assign(node); err == nil {
		t.Fatal("expected depth error")
	}
}

func TestPayloadTypeMismatchIsZeroNotPanic(t *testing.T) {
	p := ToPayload(WirePayload{T: suigo.PayloadInt, V: "not a number"})
	if p.Int != 0 || p.Tag != suigo.PayloadInt {
		t.Errorf("mismatch should yield zero: %+v", p)
	}
}
