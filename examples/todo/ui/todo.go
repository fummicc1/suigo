// Package ui は TODO の画面定義。v0.1 で最も難しい 2 点——行の同一性（List + ForEach の明示 ID）と
// 入力の確定（TextField のローカル下書き + コミット）——を通すためのサンプル。
package ui

import (
	"strconv"

	"github.com/fummicc1/suigo/suigo"
)

type Item struct {
	ID    int
	Title string
	Done  bool
}

// Model はホットリロード時に JSON で引き継がれる。
type Model struct {
	Items  []Item
	Draft  string
	NextID int
}

func init() {
	suigo.Register(Model{NextID: 1}, View)
}

func View(m Model, s *suigo.Store[Model]) suigo.Node {
	done := 0
	for _, it := range m.Items {
		if it.Done {
			done++
		}
	}

	return suigo.VStack(
		suigo.HStack(
			suigo.TextField("新しいタスク", m.Draft, func(v string) {
				s.Update(func(m *Model) { m.Draft = v })
			}).CommitOn(suigo.CommitOnSubmit),
			suigo.Button("追加", func() {
				s.Update(func(m *Model) {
					if m.Draft == "" {
						return
					}
					m.Items = append(m.Items, Item{ID: m.NextID, Title: m.Draft})
					m.NextID++
					m.Draft = ""
				})
			}).Disabled(m.Draft == ""),
		).Spacing(8),

		suigo.Divider(),

		suigo.List(
			suigo.ForEach(m.Items, func(it Item) string { return "todo-" + strconv.Itoa(it.ID) },
				func(it Item) suigo.Node {
					return suigo.HStack(
						suigo.Toggle(it.Title, it.Done, func(on bool) {
							s.Update(func(m *Model) {
								for i := range m.Items {
									if m.Items[i].ID == it.ID {
										m.Items[i].Done = on
									}
								}
							})
						}),
						suigo.Spacer(),
						suigo.Button("削除", func() {
							s.Update(func(m *Model) {
								kept := m.Items[:0]
								for _, x := range m.Items {
									if x.ID != it.ID {
										kept = append(kept, x)
									}
								}
								m.Items = kept
							})
						}).ForegroundColor(suigo.Red),
					)
				}),
		),

		suigo.Text(strconv.Itoa(done)+" / "+strconv.Itoa(len(m.Items))+" 件完了").
			Font(suigo.Caption).
			ForegroundColor(suigo.Secondary),
	).Spacing(12).Padding(16)
}
