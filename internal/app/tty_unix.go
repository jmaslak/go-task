//go:build unix

package app

import (
	"fmt"
	"os"
	"syscall"
)

// ttyID identifies the terminal standard input is connected to, or the empty
// string when it is not a terminal.
func ttyID() string {
	if !isTerminal(os.Stdin) {
		return ""
	}

	info, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	return fmt.Sprintf("%d", stat.Rdev)
}
