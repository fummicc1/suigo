package suigo

// ForEach は items の各要素を body で Node に変換し、ForEach ノードの子として並べる。
//
// id は必須引数である。省略した呼び出しはコンパイルが通らないため、
// FR-2.3（ForEach の行には明示 ID を必須とする）を規約ではなく型で担保している。
//
// ForEach 自体もワイヤ上に 1 ノードとして残す。行を親へ平坦化すると、
// 後続の兄弟の構造インデックスが行数に依存して変わり、SwiftUI がビューを作り直してしまう
// （docs/wire-format.md §6.1）。
func ForEach[T any](items []T, id func(T) string, body func(T) Node) Node {
	rows := make([]Node, 0, len(items))
	for _, it := range items {
		rows = append(rows, body(it).ID(id(it)))
	}
	return newNode(TypeForEach, rows...)
}
