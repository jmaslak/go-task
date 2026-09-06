package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// ctrlL redraws the screen rather than exiting the monitor.
const ctrlL = 0x0c

// Monitor displays the task list, refreshing it until a key is pressed.
func (a *App) Monitor(opts ListOptions) error {
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return errors.New("terminal not supported")
	}
	defer term.Restore(fd, state)

	// Raw mode leaves it to the writer to return the carriage.
	out := a.Out
	a.Out = &crlfWriter{w: out}
	defer func() { a.Out = out }()

	keys := make(chan byte, 1)
	go func() {
		defer close(keys)
		buf := make([]byte, 1)
		for {
			if _, err := os.Stdin.Read(buf); err != nil {
				return
			}
			keys <- buf[0]
		}
	}()

	screen := ""
	if screen, err = a.monitorDraw(opts, screen, true); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case key, ok := <-keys:
			if !ok {
				return nil
			}
			if key != ctrlL {
				a.printf("%s\n\nExiting.\n", a.Config.Reset)
				return nil
			}
			if screen, err = a.monitorDraw(opts, screen, true); err != nil {
				return err
			}
		case <-ticker.C:
			if screen, err = a.monitorDraw(opts, screen, false); err != nil {
				return err
			}
		}
	}
}

// monitorDraw redraws the screen if the task list has changed, or if the
// redraw was asked for. It returns what is now on the screen.
func (a *App) monitorDraw(opts ListOptions, previous string, force bool) (string, error) {
	rows, cols, err := terminalSize()
	if err != nil {
		return previous, errors.New("terminal not supported")
	}

	// Two rows go to the clock and the blank line under it, one to the
	// prompt at the bottom.
	const headerRows = 2
	opts.Max = max(rows-1-headerRows, 1)
	opts.Width = cols - 1

	var screen strings.Builder
	if a.Config.Monitor.DisplayTime {
		now := time.Now()
		fmt.Fprintf(&screen, "%s local / %s UTC\n\n",
			now.Format(ctimeLayout), now.UTC().Format(ctimeLayout))
	}

	var list string
	err = a.Store.WithLock(func() error {
		var err error
		list, err = a.taskList(opts)
		return err
	})
	if err != nil {
		return previous, err
	}
	screen.WriteString(list)

	if screen.String() == previous && !force {
		return previous, nil
	}

	if err := a.Store.WithLock(a.updateTaskLog); err != nil {
		return previous, err
	}

	a.clearScreen()
	if list == "" {
		a.printf("\n  --> %sCONGRATS YOU HAVE AN EMPTY LIST!%s <--\n\n",
			a.Config.PromptInfoColor, a.Config.Reset)
	}
	a.printf("%s%s", screen.String(), a.Config.Reset)
	a.printf("     ...Type any character to exit...  ")

	return screen.String(), nil
}

// crlfWriter returns the carriage along with each newline, which a terminal
// in raw mode will not do on its own.
type crlfWriter struct {
	w io.Writer
}

func (c *crlfWriter) Write(p []byte) (int, error) {
	if _, err := c.w.Write([]byte(strings.ReplaceAll(string(p), "\n", "\r\n"))); err != nil {
		return 0, err
	}
	return len(p), nil
}
