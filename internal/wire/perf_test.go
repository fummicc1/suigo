package wire

import (
	"testing"
	"time"

	"github.com/fummicc1/suigo/suigo"
)

// 170 行 = 511 ノード。PRE-3 と同じ規模。
func tree511() suigo.Node {
	kids := make([]suigo.Node, 170)
	for i := range kids {
		kids[i] = suigo.HStack(suigo.Text("item"), suigo.Button("done", func() {}))
	}
	return suigo.VStack(kids...)
}

func encode(t testing.TB, n suigo.Node) []byte {
	t.Helper()
	a, err := suigo.Assign(n)
	if err != nil {
		t.Fatal(err)
	}
	b, err := (JSON{}).Encode(RenderFrame{Type: "render", Seq: 1, Tree: FromAssigned(a.Root)})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TP-006: 採番 + 符号化が 511 ノードで予算内。実測は約 1 ms（採番 0.43 + 符号化 0.57）。
// CI マシン差を吸収するため閾値は 50 倍に置く。落ちたら性能ではなく計算量の退化を疑う。
func TestAssignAndEncode511UnderBudget(t *testing.T) {
	n := tree511()
	encode(t, n) // warm-up
	var best time.Duration
	for i := 0; i < 5; i++ {
		t0 := time.Now()
		encode(t, n)
		if d := time.Since(t0); i == 0 || d < best {
			best = d
		}
	}
	t.Logf("TP-006 assign+encode 511 nodes: best of 5 = %s (measured ~1ms, limit 50ms)", best)
	if best > 50*time.Millisecond {
		t.Fatalf("assign+encode took %s for 511 nodes; limit 50ms", best)
	}
}

func BenchmarkAssignAndEncode511(b *testing.B) {
	n := tree511()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		encode(b, n)
	}
}
