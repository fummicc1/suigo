//go:build suigodev

package main

import (
	"log"

	"github.com/fummicc1/suigo/suigo"

	_ "github.com/fummicc1/suigo/devserver"
	_ "github.com/fummicc1/suigo/examples/todo/ui"
)

func main() {
	if err := suigo.Run(); err != nil {
		log.Fatal(err)
	}
}
