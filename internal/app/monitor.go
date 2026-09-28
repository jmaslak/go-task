package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/jmaslak/go-task/task"
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

	var screen monitorScreen
	if screen, err = a.monitorDraw(opts, screen, true); err != nil {
		return err
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	keys := readKey()
	for {
		select {
		case key, ok := <-keys:
			if !ok {
				// The terminal has nothing more to give.
				return nil
			}
			if key != ctrlL {
				a.printf("%s\n\nExiting.\n", a.Config.Reset)
				return nil
			}
			if screen, err = a.monitorDraw(opts, screen, true); err != nil {
				return err
			}
			keys = readKey()
		case <-ticker.C:
			if screen, err = a.monitorDraw(opts, screen, false); err != nil {
				return err
			}
		}
	}
}

// monitorScreen is what the monitor last put on the terminal.
type monitorScreen struct {
	// clock is the time as it was last displayed, empty when the clock is
	// switched off.
	clock string
	// list is the task listing as it was last displayed.
	list string
	// locked records that the listing could not be refreshed because
	// another process was holding the task directory.
	locked bool
	// drawn is set once anything at all has been displayed.
	drawn bool
}

// monitorDraw redraws the screen if the task list has changed, or if the
// redraw was asked for. It returns what is now on the screen.
func (a *App) monitorDraw(opts ListOptions, previous monitorScreen, force bool) (monitorScreen, error) {
	rows, cols, err := terminalSize()
	if err != nil {
		return previous, errors.New("terminal not supported")
	}

	// Two rows go to the clock and the blank line under it, one to the
	// prompt at the bottom.
	const headerRows = 2
	opts.Max = max(rows-1-headerRows, 1)
	opts.Width = cols - 1

	current := previous
	current.locked = false
	current.clock = ""
	if a.Config.Monitor.DisplayTime {
		now := time.Now()
		current.clock = fmt.Sprintf("%s local / %s UTC\n\n",
			now.Format(ctimeLayout), now.UTC().Format(ctimeLayout))
	}

	// Waiting for the lock would leave the monitor unable to answer a
	// keystroke for as long as another process held the task directory - a
	// peer sitting in an editor holds it for as long as its user takes - so
	// only the first draw waits for the directory at all. A refresh that
	// cannot have it keeps the listing it already has and says that the
	// listing may be out of date, which the next second can put right.
	withLock := a.Store.TryWithLock
	if !previous.drawn {
		withLock = a.Store.WithLock
	}

	err = withLock(func() error {
		list, err := a.taskList(opts)
		if err != nil {
			return err
		}
		current.list = list

		// The task log records what this terminal has been shown, so it
		// is only written when the screen is about to change.
		if current == previous && !force {
			return nil
		}
		return a.updateTaskLog()
	})
	switch {
	case errors.Is(err, task.ErrLocked):
		current.locked = true
	case err != nil:
		return previous, err
	}

	if current == previous && !force {
		return previous, nil
	}

	a.clearScreen()
	if current.list == "" && !current.locked {
		a.printf("\n  --> %sCONGRATS YOU HAVE AN EMPTY LIST!%s <--\n\n",
			a.Config.PromptInfoColor, a.Config.Reset)
	}
	a.printf("%s%s%s", current.clock, current.list, a.Config.Reset)

	// The note about the lock shares the prompt's line: an extra line of its
	// own would push the listing off the top of the screen.
	if current.locked {
		a.printf("     ...Waiting for another task process...")
	}
	a.printf("     ...Type any character to exit...  ")

	current.drawn = true
	return current, nil
}

// readKey reads one keystroke from the terminal, delivering it on the returned
// channel and closing the channel once there is nothing more to read.
//
// The reader ends with the keystroke it read rather than waiting on the next
// one, so that the monitor does not leave a read of the terminal outstanding
// for a later prompt to lose its input to. A monitor that gives up with an
// error can still leave one keystroke to be swallowed, which is the most it
// ever costs.
func readKey() <-chan byte {
	keys := make(chan byte, 1)

	go func() {
		defer close(keys)

		buf := make([]byte, 1)
		if _, err := os.Stdin.Read(buf); err != nil {
			return
		}
		keys <- buf[0]
	}()

	return keys
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
