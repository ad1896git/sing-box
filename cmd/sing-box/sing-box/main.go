//go:build !generate

// Command sing-box is the CLI. The commands live in the importable package
// github.com/sagernet/sing-box/cmd/sing-box (boxmain), which nekobox_core uses.
package main

import boxmain "github.com/sagernet/sing-box/cmd/sing-box"

func main() {
	boxmain.Main()
}
