package app

import (
	"os"

	"golang.org/x/term"
)

// isTerminal reports whether a file is connected to a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// terminalSize returns the size of the controlling terminal in rows and
// columns.
func terminalSize() (rows, cols int, err error) {
	cols, rows, err = term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0, 0, err
	}
	return rows, cols, nil
}
