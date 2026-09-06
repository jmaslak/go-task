// Package app implements the user facing behavior of the task application:
// the commands, the prompts, and the rendering of tasks to the terminal.
package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/jmaslak/go-task/internal/config"
	"github.com/jmaslak/go-task/internal/task"
)

// Prompt fragments shared by every prompt the application shows.
const (
	promptPrefix = "[task]"
	promptSuffix = "> "
)

// ErrAborted reports that the user declined at a prompt, or that input ran
// out. It is not a failure: the caller stops what it was doing and exits
// cleanly.
var ErrAborted = errors.New("aborted")

// App ties together the task store, the configuration, and the terminal.
type App struct {
	Store  *task.Store
	Config *config.Config

	In  *bufio.Reader
	Out io.Writer
	Err io.Writer

	// Interactive is true when input comes from a terminal. It gates the
	// checks that keep a stale task list from being acted on.
	Interactive bool
	// Quiet suppresses prompt text. Tests use it to keep output readable.
	Quiet bool
	// CheckFreshness requires a task list to have been displayed in this
	// terminal before tasks are modified by number.
	CheckFreshness bool
}

// New returns an application reading from and writing to the process's own
// standard streams.
func New(store *task.Store, cfg *config.Config) *App {
	return &App{
		Store:          store,
		Config:         cfg,
		In:             bufio.NewReader(os.Stdin),
		Out:            os.Stdout,
		Err:            os.Stderr,
		Interactive:    isTerminal(os.Stdin),
		CheckFreshness: true,
	}
}

// printf writes to the application's output stream.
func (a *App) printf(format string, args ...any) {
	fmt.Fprintf(a.Out, format, args...)
}

// errorf writes to the application's error stream.
func (a *App) errorf(format string, args ...any) {
	fmt.Fprintf(a.Err, format, args...)
}

// prompt writes a prompt and reads one line of input. It reports false once
// input is exhausted.
func (a *App) prompt(text string) (string, bool) {
	if !a.Quiet {
		a.printf("%s%s%s", a.Config.PromptColor, text, a.Config.Reset)
	}

	line, err := a.In.ReadString('\n')
	if line == "" && err != nil {
		return "", false
	}
	return strings.TrimRight(line, "\n"), true
}

// stringPrompt asks for a non-empty line of text.
func (a *App) stringPrompt(text string) (string, error) {
	a.printf("\n")
	for {
		line, ok := a.prompt(text)
		if !ok {
			return "", ErrAborted
		}
		if line != "" {
			return line, nil
		}

		a.printf("Invalid input, please try again\n\n")
	}
}

// yesNoPrompt asks a yes or no question. An empty answer means yes, and input
// that runs out means no.
func (a *App) yesNoPrompt(text string) bool {
	if !a.Quiet {
		a.printf("\n")
	}

	for {
		line, ok := a.prompt(text)
		if !ok {
			return false
		}

		switch strings.ToLower(line) {
		case "", "y", "yes":
			return true
		case "n", "no":
			return false
		}

		if !a.Quiet {
			a.printf("Invalid choice, please try again\n\n")
		}
	}
}

// menuPrompt shows a numbered menu and returns the value of the choice made.
func (a *App) menuPrompt(text string, choices []menuChoice) (string, error) {
	width := len(fmt.Sprint(len(choices)))
	for i, choice := range choices {
		a.printf("%s%*d.%s %s\n",
			a.Config.PromptBoldColor, width, i+1, a.Config.PromptInfoColor, choice.description)
	}
	a.printf("\n")

	for {
		line, ok := a.prompt(text)
		if !ok {
			return "", ErrAborted
		}

		if index, err := strconv.Atoi(line); err == nil && index >= 1 && index <= len(choices) {
			return choices[index-1].value, nil
		}

		a.printf("Invalid choice, please try again\n\n")
	}
}

// choicePrompt asks for one of a set of answers, without listing them.
func (a *App) choicePrompt(text string, choices []string) (string, error) {
	a.printf("\n")
	for {
		line, ok := a.prompt(text)
		if !ok {
			return "", ErrAborted
		}
		if slices.Contains(choices, line) {
			return line, nil
		}

		a.printf("Invalid choice, please try again\n\n")
	}
}

// askForTag asks for a tag, which may not be empty or contain whitespace.
func (a *App) askForTag() (string, error) {
	for {
		tag, err := a.stringPrompt(fmt.Sprintf("%s Please enter tag %s", promptPrefix, promptSuffix))
		if err != nil {
			return "", err
		}
		if len(strings.Fields(tag)) == 1 {
			return tag, nil
		}

		a.printf("Tag must not be empty or contain any spaces\n\n")
	}
}

// askForDate asks for a date in YYYY-MM-DD form.
func (a *App) askForDate(text string) (task.Date, error) {
	for {
		answer, err := a.stringPrompt(text)
		if err != nil {
			return task.Date{}, err
		}

		day, err := task.ParseDate(answer)
		if err == nil {
			return day, nil
		}

		a.printf("Date format is incorrect\n\n")
	}
}

// menuChoice is one entry of the interactive command menu.
type menuChoice struct {
	description string
	value       string
}
