// Package ui はカウンターの画面定義。dev と embedded の両方から同じコードが使われる。
package ui

import (
	"strconv"

	"github.com/fummicc1/suigo/suigo"
)

// Model はホットリロード時に JSON で引き継がれる。エクスポートされたフィールドだけが残る。
type Model struct {
	Count int
}

func init() {
	suigo.Register(Model{}, View)
}

// View は純粋関数。副作用は持たず、モデルからツリーを作るだけ。
func View(m Model, s *suigo.Store[Model]) suigo.Node {
	return suigo.VStack(
		suigo.Text("suigo counter").Font(suigo.Caption).ForegroundColor(suigo.Secondary),
		suigo.Text(strconv.Itoa(m.Count)).Font(suigo.LargeTitle).ForegroundColor(suigo.Blue),
		suigo.HStack(
			suigo.Button("−", func() { s.Update(func(m *Model) { m.Count-- }) }).Padding(12),
			suigo.Button("+", func() { s.Update(func(m *Model) { m.Count++ }) }).Padding(12),
		).Spacing(24),
		suigo.Button("Reset", func() { s.Update(func(m *Model) { m.Count = 0 }) }).
			Font(suigo.Caption).
			Disabled(m.Count == 0),
	).Spacing(16).Padding(24)
}
