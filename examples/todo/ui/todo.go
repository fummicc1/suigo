// Package ui は TODO の画面定義。v0.1 で最も難しい 2 点——行の同一性（List + ForEach の明示 ID）と
// 入力の確定（TextField のローカル下書き + コミット）——を通すためのサンプル。
//
// 入力欄は 3 つあり、コミット契機が違う。
//   - リスト名:     blur（別の欄へ移るなどフォーカスを失ったときに確定。打っている間は送らない）
//   - 新しいタスク: submit（return で確定）
//   - 絞り込み:     debounce 300ms（打つのを止めると勝手に確定。return は要らない）
package ui

import (
	"strconv"
	"strings"

	"github.com/fummicc1/suigo/suigo"
)

type Item struct {
	ID    int
	Title string
	Done  bool
}

// Model はホットリロード時に JSON で引き継がれる。
type Model struct {
	Title  string
	Items  []Item
	Draft  string
	Filter string
	NextID int
}

func init() {
	suigo.Register(Model{Title: "やること", NextID: 1}, View)
}

func View(m Model, s *suigo.Store[Model]) suigo.Node {
	done := 0
	for _, it := range m.Items {
		if it.Done {
			done++
		}
	}
	visible := m.Items
	if m.Filter != "" {
		visible = visible[:0:0]
		for _, it := range m.Items {
			if strings.Contains(it.Title, m.Filter) {
				visible = append(visible, it)
			}
		}
	}

	children := []suigo.Node{
		// Title が空でも見出しは消さない。先頭で出し入れすると後続の兄弟の構造 ID がずれ、下書き中の TextField や List が作り直される。
		suigo.Text(m.Title).Font(suigo.Title2),
		suigo.TextField("リスト名", m.Title, func(v string) {
			s.Update(func(m *Model) { m.Title = v })
		}).CommitOn(suigo.CommitOnBlur),

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

		suigo.TextField("絞り込み", m.Filter, func(v string) {
			s.Update(func(m *Model) { m.Filter = v })
		}).CommitOn(suigo.CommitDebounce(300)),

		suigo.Divider(),

		suigo.List(
			suigo.ForEach(visible, func(it Item) string { return "todo-" + strconv.Itoa(it.ID) },
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

		suigo.Text(strconv.Itoa(done) + " / " + strconv.Itoa(len(m.Items)) + " 件完了").
			Font(suigo.Caption).
			ForegroundColor(suigo.Secondary),
	}
	if m.Filter != "" {
		children = append(children,
			suigo.Text("「"+m.Filter+"」で "+strconv.Itoa(len(visible))+" 件").
				Font(suigo.Caption).
				ForegroundColor(suigo.Secondary))
	}

	return suigo.VStack(children...).Spacing(12).Padding(16)
}
