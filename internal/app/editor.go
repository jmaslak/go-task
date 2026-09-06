package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// editorPrompt heads the scratch file handed to the editor, telling the user
// where the note starts.
const editorPrompt = "Please enter any notes that should appear for this task below this line."

// editorRule separates the prompt from the note itself.
var editorRule = strings.Repeat("-", 72)

// noteFromUser collects a note in the user's editor. It returns an empty note
// when the user declines to write one.
func (a *App) noteFromUser() (string, error) {
	// The editor takes over the screen, so ask before it wipes away the
	// task that is being annotated.
	if !a.yesNoPrompt(fmt.Sprintf("%s Add a Note to This Task [Y/n]? %s", promptPrefix, promptSuffix)) {
		return "", nil
	}

	temp, err := os.CreateTemp("", "task-note-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())

	if _, err := fmt.Fprintf(temp, "%s\n%s\n", editorPrompt, editorRule); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}

	argv := expandCommand(a.Config.EditorCommand, map[string]string{"%FILENAME%": temp.Name()})
	if len(argv) == 0 {
		return "", fmt.Errorf("no editor command configured")
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = a.Err
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("editor %s: %w", argv[0], err)
	}

	edited, err := os.ReadFile(temp.Name())
	if err != nil {
		return "", err
	}

	return trimNote(string(edited)), nil
}

// trimNote strips the header the editor was handed, along with the blank
// lines around what the user wrote.
func trimNote(contents string) string {
	lines := strings.Split(strings.TrimSuffix(contents, "\n"), "\n")

	if len(lines) > 0 && strings.TrimSpace(lines[0]) == editorPrompt {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == editorRule {
		lines = lines[1:]
	}

	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}
