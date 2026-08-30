//go:build suigodev

// dev 用の入口。`suigo dev` がこれをビルドして子プロセスとして起動する。
package main

import (
	"log"

	"github.com/fummicc1/suigo/suigo"

	_ "github.com/fummicc1/suigo/devserver"     // suigo.Run を dev サーバに接続する
	_ "github.com/fummicc1/suigo/examples/counter/ui" // init() で View を登録する
)

func main() {
	if err := suigo.Run(); err != nil {
		log.Fatal(err)
	}
}
