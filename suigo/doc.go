// Package suigo は Go で UI を記述するための宣言的 DSL を提供する。
//
// 記述したツリーは JSON に符号化され、Swift 側のインタープリタが
// 本物の SwiftUI View として描画する。
//
//	suigo.VStack(
//	    suigo.Text(fmt.Sprint(m.Count)).Font(suigo.Title),
//	    suigo.Button("Tap", func() { s.Update(func(m *Model) { m.Count++ }) }).Padding(8),
//	).Spacing(12)
//
// 本パッケージは pure Go であり、macOS 無しでテストできる。
// 描画・境界・トランスポートには依存しない。
package suigo
