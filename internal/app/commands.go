package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jmaslak/go-task/internal/task"
)

// ErrStaleTaskList reports that the task numbers this terminal last saw no
// longer describe the tasks on disk.
var ErrStaleTaskList = errors.New("task list is stale")

// NewOptions are the settings a task can be created with.
type NewOptions struct {
	// ExpireToday closes the task at the end of the day it was created.
	ExpireToday bool
	// MaturityDate holds the task back until that day.
	MaturityDate task.Date
	// Tag is applied to the new task.
	Tag string
}

// NewTask creates a task and returns its number. An empty title is asked for,
// along with an optional note.
func (a *App) NewTask(title string, opts NewOptions) (int, error) {
	if opts.ExpireToday && !opts.MaturityDate.IsZero() {
		return 0, errors.New("cannot use both the --expire-today and --maturity-date options simultaneously")
	}

	number := 0
	err := a.Store.WithLock(func() error {
		var err error
		number, err = a.createTask(title, opts)
		return err
	})

	return number, err
}

// createTask does the work of NewTask, with the task directory already
// locked.
func (a *App) createTask(title string, opts NewOptions) (int, error) {
	number, err := a.Store.NextNumber()
	if err != nil {
		return 0, err
	}

	// A title given on the command line means the whole task was given on
	// the command line; only prompt when it was not.
	interactive := title == ""
	if interactive {
		title, err = a.stringPrompt(fmt.Sprintf("%s Enter Task Subject %s", promptPrefix, promptSuffix))
		if err != nil {
			return 0, err
		}
	}

	title = normalizeTitle(title)
	if title == "" {
		a.printf("Blank subject, exiting.\n")
		return 0, ErrAborted
	}
	a.printf("\n")

	var note string
	if interactive {
		if note, err = a.noteFromUser(); err != nil {
			return 0, err
		}

		if !a.yesNoPrompt(fmt.Sprintf("%s Save This Task [Y/n]? %s", promptPrefix, promptSuffix)) {
			a.printf("Aborting.\n")
			return 0, ErrAborted
		}
	}

	t := task.New(number, title)
	if opts.Tag != "" {
		if _, err := t.AddTag(opts.Tag); err != nil {
			return 0, err
		}
	}
	if !opts.MaturityDate.IsZero() {
		if err := t.SetNotBefore(opts.MaturityDate); err != nil {
			return 0, err
		}
	}
	if opts.ExpireToday {
		if err := t.SetExpires(task.Today()); err != nil {
			return 0, err
		}
	}
	if note != "" {
		if err := t.AddNote(note); err != nil {
			return 0, err
		}
	}

	if err := a.Store.Save(t); err != nil {
		return 0, err
	}

	a.printf("Created task %d\n", number)
	return number, nil
}

// List displays the task list through the pager.
func (a *App) List(opts ListOptions) error {
	return a.Store.WithLock(func() error {
		if err := a.updateTaskLog(); err != nil {
			return err
		}

		list, err := a.taskList(opts)
		if err != nil {
			return err
		}

		return a.displayWithPager("Tasklist", list)
	})
}

// Show displays a single task through the pager.
func (a *App) Show(number int) error {
	return a.Store.WithLock(func() error {
		t, err := a.Store.Task(number)
		if err != nil {
			return a.notFound(number, err)
		}

		return a.displayWithPager(fmt.Sprintf("Task %d", number), a.show(t))
	})
}

// AddNote appends a note to a task. An empty note is collected from the
// user's editor.
func (a *App) AddNote(number int, note string) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("add note"); err != nil {
			return err
		}

		return a.addNote(number, note)
	})
}

// addNote appends a note without checking the freshness of the task list, so
// that it can be reused by commands that have already checked.
func (a *App) addNote(number int, note string) error {
	t, err := a.Store.Task(number)
	if err != nil {
		return a.notFound(number, err)
	}

	if note == "" {
		if err := a.displayWithPager(fmt.Sprintf("Task %d", number), a.show(t)); err != nil {
			return err
		}
		if note, err = a.noteFromUser(); err != nil {
			return err
		}
		if note == "" {
			a.printf("Not adding note\n")
			return nil
		}

		if !a.yesNoPrompt(fmt.Sprintf("%s Save This Task [Y/n]? %s", promptPrefix, promptSuffix)) {
			a.printf("Aborting.\n")
			return ErrAborted
		}
	}

	if err := t.AddNote(note); err != nil {
		return err
	}
	if err := a.Store.Save(t); err != nil {
		return err
	}

	a.printf("Updated task %d\n", number)
	return nil
}

// CloseTask closes a task, moving it into the done directory. The user is
// given the chance to add a closing note first.
func (a *App) CloseTask(number int) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("close task"); err != nil {
			return err
		}

		if err := a.addNote(number, ""); err != nil {
			return err
		}

		return a.closeTask(number, true)
	})
}

// closeTask archives a task, optionally renumbering the tasks left behind.
func (a *App) closeTask(number int, coalesce bool) error {
	if err := a.Store.Archive(number); err != nil {
		return a.notFound(number, err)
	}
	a.printf("Closed %05d\n", number)

	if coalesce {
		return a.Store.Coalesce()
	}
	return nil
}

// Move renumbers a task, shifting the tasks in between out of its way.
func (a *App) Move(from, to int) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("move task"); err != nil {
			return err
		}

		return a.Store.Move(from, to)
	})
}

// Retitle changes a task's title, recording the change as a note.
func (a *App) Retitle(number int, title string) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("retitle"); err != nil {
			return err
		}

		number, err := a.resolveNumber(number)
		if err != nil {
			return err
		}
		if title == "" {
			title, err = a.stringPrompt(fmt.Sprintf("%s Please enter the new title %s", promptPrefix, promptSuffix))
			if err != nil {
				return err
			}
		}

		t, err := a.Store.Task(number)
		if err != nil {
			return a.notFound(number, err)
		}

		note := fmt.Sprintf("Title changed from:\n  %s\nTo:\n  %s", t.Title, title)
		if err := t.AddNote(note); err != nil {
			return err
		}
		if err := t.SetTitle(title); err != nil {
			return err
		}

		return a.Store.Save(t)
	})
}

// SetExpiration sets the last day a task is relevant. A zero day is asked
// for.
func (a *App) SetExpiration(number int, day task.Date) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("set expiration"); err != nil {
			return err
		}

		number, err := a.resolveNumber(number)
		if err != nil {
			return err
		}
		if day.IsZero() {
			day, err = a.askForDate(fmt.Sprintf(
				"%s Please enter the last valid day for this task %s", promptPrefix, promptSuffix))
			if err != nil {
				return err
			}
		}
		if day.Before(task.Today()) {
			return errors.New("date cannot be before today")
		}

		return a.setExpiration(number, day)
	})
}

// setExpiration records a new expiration date without prompting.
func (a *App) setExpiration(number int, day task.Date) error {
	t, err := a.Store.Task(number)
	if err != nil {
		return a.notFound(number, err)
	}

	note := fmt.Sprintf("Added expiration date: %s", day)
	if !t.Expires.IsZero() {
		note = fmt.Sprintf("Updated expiration date from %s to %s", t.Expires, day)
	}

	if err := t.AddNote(note); err != nil {
		return err
	}
	if err := t.SetExpires(day); err != nil {
		return err
	}

	return a.Store.Save(t)
}

// SetMaturity sets the first day a task is displayed. A zero day is asked
// for.
func (a *App) SetMaturity(number int, day task.Date) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("set maturity date"); err != nil {
			return err
		}

		number, err := a.resolveNumber(number)
		if err != nil {
			return err
		}
		if day.IsZero() {
			day, err = a.askForDate(fmt.Sprintf(
				"%s Please enter the day to start displaying this task %s", promptPrefix, promptSuffix))
			if err != nil {
				return err
			}
		}
		if !day.After(task.Today()) {
			return errors.New("date cannot be before or equal to today")
		}

		return a.setMaturity(number, day)
	})
}

// setMaturity records a new maturity date without prompting.
func (a *App) setMaturity(number int, day task.Date) error {
	t, err := a.Store.Task(number)
	if err != nil {
		return a.notFound(number, err)
	}

	note := fmt.Sprintf("Added not-before date: %s", day)
	if !t.NotBefore.IsZero() {
		note = fmt.Sprintf("Updated not-before date from %s to %s", t.NotBefore, day)
	}

	if err := t.AddNote(note); err != nil {
		return err
	}
	if err := t.SetNotBefore(day); err != nil {
		return err
	}

	return a.Store.Save(t)
}

// SetFrequency sets how many days apart a task is displayed. A frequency of
// zero is asked for.
func (a *App) SetFrequency(number int, frequency int64) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("set display frequency"); err != nil {
			return err
		}

		number, err := a.resolveNumber(number)
		if err != nil {
			return err
		}
		for frequency < 1 {
			answer, err := a.stringPrompt(fmt.Sprintf(
				"%s Please enter desired days apart for task display %s", promptPrefix, promptSuffix))
			if err != nil {
				return err
			}
			if _, err := fmt.Sscanf(answer, "%d", &frequency); err != nil || frequency < 1 {
				frequency = 0
				a.printf("Must be an integer >= 1\n\n")
			}
		}

		t, err := a.Store.Task(number)
		if err != nil {
			return a.notFound(number, err)
		}

		note := fmt.Sprintf("Added display frequency of every %s", inDays(frequency))
		if t.DisplayFrequency != 0 {
			note = fmt.Sprintf("Updated display frequency from every %s to every %s",
				inDays(t.DisplayFrequency), inDays(frequency))
		}

		if err := t.AddNote(note); err != nil {
			return err
		}
		if err := t.SetDisplayFrequency(frequency); err != nil {
			return err
		}

		return a.Store.Save(t)
	})
}

// AddTag tags a task. Adding a tag the task already carries does nothing.
func (a *App) AddTag(number int, tag string) error {
	return a.changeTag(number, tag, true)
}

// RemoveTag removes a tag from a task. Removing a tag the task does not carry
// does nothing.
func (a *App) RemoveTag(number int, tag string) error {
	return a.changeTag(number, tag, false)
}

// changeTag adds or removes a tag, recording the change as a note.
func (a *App) changeTag(number int, tag string, add bool) error {
	return a.Store.WithLock(func() error {
		if err := a.requireFreshTaskLog("set tag"); err != nil {
			return err
		}

		number, err := a.resolveNumber(number)
		if err != nil {
			return err
		}
		if tag == "" {
			if tag, err = a.askForTag(); err != nil {
				return err
			}
		}

		t, err := a.Store.Task(number)
		if err != nil {
			return a.notFound(number, err)
		}

		var changed bool
		var note string
		if add {
			changed, err = t.AddTag(tag)
			note = "Added tag " + tag
		} else {
			changed, err = t.RemoveTag(tag)
			note = "Removed tag " + tag
		}
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}

		if err := t.AddNote(note); err != nil {
			return err
		}

		return a.Store.Save(t)
	})
}

// Expire closes every task whose expiration date has passed.
func (a *App) Expire() error {
	return a.Store.WithLock(func() error {
		tasks, err := a.Store.Tasks()
		if err != nil {
			return err
		}

		today := task.Today()
		for _, t := range tasks {
			if t.Expires.IsZero() || !t.Expires.Before(today) {
				continue
			}

			if err := t.AddNote("Task expired, closed."); err != nil {
				return err
			}
			if err := a.Store.Save(t); err != nil {
				return err
			}
			if err := a.closeTask(t.Number, false); err != nil {
				return err
			}
		}

		return a.Store.Coalesce()
	})
}

// Coalesce closes the gaps in the task numbering.
func (a *App) Coalesce() error {
	if err := a.Store.Coalesce(); err != nil {
		return err
	}

	a.printf("Coalesced tasks\n")
	return nil
}

// resolveNumber returns the task number to act on, asking for one when it was
// not given.
func (a *App) resolveNumber(number int) (int, error) {
	if number > 0 {
		return number, nil
	}
	return a.AskTaskNumber("modify")
}

// notFound turns a missing task into the message the user expects, leaving
// other errors alone.
func (a *App) notFound(number int, err error) error {
	if errors.Is(err, task.ErrNotFound) {
		a.errorf("Could not locate task number %d\n", number)
	}
	return err
}

// normalizeTitle trims a title and flattens the tabs in it, so that it stays
// on one line of the task file.
func normalizeTitle(title string) string {
	title = strings.ReplaceAll(title, "\t", " ")
	return strings.TrimSpace(title)
}

// inDays renders a number of days with the right plural.
func inDays(days int64) string {
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
