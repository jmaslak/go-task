package app

import (
	"fmt"
	"strconv"
)

// menuChoices are the commands offered when the application is run with no
// arguments at all.
var menuChoices = []menuChoice{
	{"Create New Task", "new"},
	{"Add a Note to a Task", "note"},
	{"View an Existing Task", "show"},
	{"List All Tasks", "list"},
	{"Monitor Task List", "monitor"},
	{"Move (Reprioritize) a Task", "move"},
	{"Close a Task", "close"},
	{"Coalesce Tasks", "coalesce"},
	{"Retitle Tasks", "retitle"},
	{"Set Task Expiration", "set-expire"},
	{"Expire Tasks", "expire"},
	{"Set Task Maturity Date", "set-maturity"},
	{"Set Task Display Frequency", "set-frequency"},
	{"Add Tag", "add-tag"},
	{"Remove Tag", "remove-tag"},
	{"Quit to Shell", "quit"},
}

// Menu asks which command to run and returns its name.
func (a *App) Menu() (string, error) {
	a.printf("%sPlease select an option...\n\n", a.Config.PromptColor)

	return a.menuPrompt(fmt.Sprintf("%s %s", promptPrefix, promptSuffix), menuChoices)
}

// AskTaskNumber asks which task a command should act on. The menu uses it for
// the commands that need a task number.
func (a *App) AskTaskNumber(purpose string) (int, error) {
	numbers, err := a.Store.Numbers()
	if err != nil {
		return 0, err
	}

	choices := make([]string, len(numbers))
	for i, number := range numbers {
		choices[i] = fmt.Sprint(number)
	}

	answer, err := a.choicePrompt(
		fmt.Sprintf("%s Please enter task number to %s %s", promptPrefix, purpose, promptSuffix),
		choices,
	)
	if err != nil {
		return 0, err
	}

	number, _ := strconv.Atoi(answer)
	return number, nil
}

// AskNumber asks for a non-negative number, such as the destination of a
// move.
func (a *App) AskNumber(text string) (int, error) {
	a.printf("\n")
	for {
		line, ok := a.prompt(text)
		if !ok {
			return 0, ErrAborted
		}

		if number, err := strconv.Atoi(line); err == nil && number >= 0 {
			return number, nil
		}

		a.printf("Invalid number, please try again\n\n")
	}
}
