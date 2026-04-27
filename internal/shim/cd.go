// Package shim emits the CD sentinel that the .zshrc wtree() function reads
// to change the parent shell's working directory.
package shim

import (
	"fmt"
	"os"
)

// SentinelPrefix is the contract with the .zshrc shim: any line on stdout
// starting with this is a `cd` request. Everything else MUST go to stderr.
const SentinelPrefix = "__WTREE_CD__:"

func PrintCD(path string) {
	fmt.Fprintln(os.Stdout, SentinelPrefix+path)
}
