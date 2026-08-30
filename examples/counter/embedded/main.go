// 本番（embedded）用の入口。scripts/build-xcframework.sh がこれを c-archive にする。
//
// main() は -buildmode=c-archive では実行されない。起動は suigo_mount() から始まり、
// init() で登録された ui パッケージのアプリに到達する（design §9.1）。
package main

import (
	_ "github.com/fummicc1/suigo/embedded"            // //export 関数
	_ "github.com/fummicc1/suigo/examples/counter/ui" // init() で View を登録する
)

func main() {}
