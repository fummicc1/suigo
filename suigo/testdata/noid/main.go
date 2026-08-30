// このファイルはコンパイルに失敗することを期待されている（TG-213）。
// ForEach の id 引数を省略した呼び出しが型エラーになることで、
// FR-2.3 が規約ではなく型で担保されていることを検証する。
package main

import "github.com/fummicc1/suigo/suigo"

func main() {
	_ = suigo.ForEach([]int{1, 2}, func(int) suigo.Node { return suigo.Text("x") })
}
