package app

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// statusFileName holds the fingerprint of the task list, along with the
// terminals that fingerprint has been displayed in.
const statusFileName = ".taskview.status"

// taskHash fingerprints the current task list. Commands that take a task
// number refuse to run when the list has changed since it was last shown,
// because the numbers the user is working from may no longer mean what they
// did.
func (a *App) taskHash() (string, error) {
	list, err := a.taskList(ListOptions{})
	if err != nil {
		return "", err
	}

	sum := sha1.Sum([]byte(list))
	return hex.EncodeToString(sum[:]), nil
}

// terminalID identifies the terminal this process is running in.
func (a *App) terminalID() string {
	return fmt.Sprintf("%d:%s", os.Getppid(), ttyID())
}

// updateTaskLog records that this terminal has seen the current task list.
func (a *App) updateTaskLog() error {
	hash, err := a.taskHash()
	if err != nil {
		return err
	}

	recorded, terminals, err := a.readTaskLog()
	if err != nil {
		return err
	}

	terminal := a.terminalID()
	if recorded != hash {
		terminals = nil
	} else if slices.Contains(terminals, terminal) {
		return nil
	}

	lines := append([]string{hash}, terminals...)
	lines = append(lines, terminal)

	return os.WriteFile(a.statusFile(), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// taskLogFresh reports whether the task numbers this terminal was last shown
// still describe the tasks on disk.
func (a *App) taskLogFresh() (bool, error) {
	if !a.Interactive || !a.CheckFreshness {
		return true, nil
	}

	hash, err := a.taskHash()
	if err != nil {
		return false, err
	}

	recorded, terminals, err := a.readTaskLog()
	if err != nil {
		return false, err
	}

	return recorded == hash && slices.Contains(terminals, a.terminalID()), nil
}

// requireFreshTaskLog lets a command that acts on a task number go ahead, or
// refuses it with ErrStaleTaskList after saying why.
func (a *App) requireFreshTaskLog(action string) error {
	fresh, err := a.taskLogFresh()
	if err != nil {
		return err
	}
	if !fresh {
		a.printf("Can't %s - task numbers may have changed since last 'task list'\n", action)
		return ErrStaleTaskList
	}

	return nil
}

// readTaskLog returns the recorded fingerprint and the terminals it was shown
// in. A missing file reads as no record at all.
func (a *App) readTaskLog() (hash string, terminals []string, err error) {
	data, err := os.ReadFile(a.statusFile())
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", nil, nil
	}

	return lines[0], lines[1:], nil
}

func (a *App) statusFile() string {
	return filepath.Join(a.Store.Dir(), statusFileName)
}
