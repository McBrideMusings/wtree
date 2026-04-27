package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// confirm prints prompt to stderr and reads a line from stdin. Empty
// (just-pressed-enter) and y/yes are accepted; anything else is rejected.
// EOF / read errors return false.
func confirm(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true
	}
	return false
}
