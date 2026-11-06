//go:build generate && generate_completions

package main

import (
	boxmain "github.com/sagernet/sing-box/cmd/sing-box"
	"github.com/sagernet/sing-box/log"
)

func main() {
	err := boxmain.GenerateCompletions()
	if err != nil {
		log.Fatal(err)
	}
}
