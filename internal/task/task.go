// Package task implements the on-disk task database: the format of a single
// task file and the operations over the directory that holds them.
package task

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// FormatVersion is the version of the task file format this package writes.
// Version 1 files lack a Task-Id header; they are upgraded when read.
const FormatVersion = 2

// ErrTrelloTask is returned by the mutating methods of a task that mirrors a
// Trello card. Such tasks are owned by Trello and are replaced, not edited,
// on the next sync.
var ErrTrelloTask = errors.New("cannot edit a task which is on Trello")

// Task is a single unit of work, stored as one file in the task directory.
type Task struct {
	// Number is the task's position in the list, which is also the numeric
	// prefix of its file name.
	Number int
	// File is the path the task was read from, empty for a task that has
	// not been written yet.
	File string

	Title   string
	Created time.Time
	// Expires is the last day on which the task is still relevant.
	Expires Date
	// NotBefore is the first day on which the task is shown.
	NotBefore Date
	// DisplayFrequency, when above one, shows the task on one day out of
	// that many. Zero means the task is always shown.
	DisplayFrequency int64
	// Tags are kept sorted and free of duplicates.
	Tags []string
	// ID is a large random-ish number identifying the task for its whole
	// life, unchanged by renumbering.
	ID *big.Int
	// TrelloID is set on tasks that mirror a Trello card.
	TrelloID string

	Notes   []Note
	Version int
}

// Note is one dated entry in a task's body.
type Note struct {
	Date time.Time
	Text string
}

// New returns a task with the mandatory fields filled in.
func New(number int, title string) *Task {
	return &Task{
		Number:  number,
		Title:   title,
		Created: time.Now(),
		ID:      NewID(),
		Version: FormatVersion,
	}
}

// headerLine matches "Field: value", the shape of every header in the file.
var headerLine = regexp.MustCompile(`^([-\w]+):\s*(.*)$`)

// bodyMarker matches the line that introduces a note, carrying its timestamp.
var bodyMarker = regexp.MustCompile(`^--- (\d+)$`)

// Unmarshal parses the contents of a task file.
func Unmarshal(data []byte) (*Task, error) {
	t := &Task{Version: FormatVersion}
	lines := splitLines(data)

	// Headers, up to the first note.
	for len(lines) > 0 && !bodyMarker.MatchString(lines[0]) {
		line := lines[0]
		lines = lines[1:]

		fields := headerLine.FindStringSubmatch(line)
		if fields == nil {
			return nil, fmt.Errorf("invalid header line: %s", line)
		}
		if err := t.setHeader(strings.ToLower(fields[1]), fields[2]); err != nil {
			return nil, err
		}
	}

	if t.Title == "" {
		return nil, errors.New("title field not found")
	}
	if t.Created.IsZero() {
		return nil, errors.New("created field not found")
	}
	if t.ID == nil {
		// A version 1 file. Give it an ID; it is written back out as a
		// version 2 file the next time it is saved.
		t.ID = NewID()
		t.Version = 1
	}

	// Notes.
	for len(lines) > 0 {
		marker := bodyMarker.FindStringSubmatch(lines[0])
		if marker == nil {
			return nil, fmt.Errorf("expected a note marker, got: %s", lines[0])
		}
		lines = lines[1:]

		seconds, err := strconv.ParseInt(marker[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid note timestamp: %w", err)
		}

		var text []string
		for len(lines) > 0 && !bodyMarker.MatchString(lines[0]) {
			text = append(text, lines[0])
			lines = lines[1:]
		}

		t.Notes = append(t.Notes, Note{
			Date: time.Unix(seconds, 0),
			Text: strings.Join(text, "\n"),
		})
	}

	return t, nil
}

// Marshal renders the task in the on-disk format.
func (t *Task) Marshal() []byte {
	var b strings.Builder

	fmt.Fprintf(&b, "Title: %s\n", t.Title)
	fmt.Fprintf(&b, "Created: %d\n", t.Created.Unix())
	fmt.Fprintf(&b, "Task-Id: %s\n", t.ID)

	if !t.Expires.IsZero() {
		fmt.Fprintf(&b, "Expires: %s\n", t.Expires)
	}
	if len(t.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(t.Tags, " "))
	}
	if !t.NotBefore.IsZero() {
		fmt.Fprintf(&b, "Not-Before: %s\n", t.NotBefore)
	}
	if t.DisplayFrequency != 0 {
		fmt.Fprintf(&b, "Display-Frequency: %d\n", t.DisplayFrequency)
	}
	if t.TrelloID != "" {
		fmt.Fprintf(&b, "Trello-ID: %s\n", t.TrelloID)
	}

	for _, note := range t.Notes {
		fmt.Fprintf(&b, "--- %d\n", note.Date.Unix())
		fmt.Fprintf(&b, "%s\n", note.Text)
	}

	return []byte(b.String())
}

// setHeader applies one parsed header line.
func (t *Task) setHeader(field, value string) error {
	switch field {
	case "title":
		t.Title = value
	case "created":
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid created timestamp: %w", err)
		}
		t.Created = time.Unix(seconds, 0)
	case "expires":
		date, err := ParseDate(value)
		if err != nil {
			return err
		}
		t.Expires = date
	case "not-before":
		date, err := ParseDate(value)
		if err != nil {
			return err
		}
		t.NotBefore = date
	case "tags":
		t.Tags = sortedTags(strings.Fields(value))
	case "task-id":
		id, ok := new(big.Int).SetString(value, 10)
		if !ok {
			return fmt.Errorf("invalid task id: %s", value)
		}
		t.ID = id
	case "display-frequency":
		frequency, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid display frequency: %w", err)
		}
		t.DisplayFrequency = frequency
	case "trello-id":
		t.TrelloID = value
	default:
		return fmt.Errorf("unknown header: %s", field)
	}
	return nil
}

// AddNote appends a note carrying the current time.
func (t *Task) AddNote(text string) error {
	if t.TrelloID != "" {
		return ErrTrelloTask
	}

	t.Notes = append(t.Notes, Note{
		Date: time.Now(),
		Text: strings.TrimSuffix(text, "\n"),
	})
	return nil
}

// SetTitle changes the task's title.
func (t *Task) SetTitle(title string) error {
	if t.TrelloID != "" {
		return ErrTrelloTask
	}
	t.Title = title
	return nil
}

// SetExpires changes the day after which the task is no longer relevant.
func (t *Task) SetExpires(day Date) error {
	if t.TrelloID != "" {
		return ErrTrelloTask
	}
	t.Expires = day
	return nil
}

// SetNotBefore changes the day on which the task starts being displayed.
func (t *Task) SetNotBefore(day Date) error {
	if t.TrelloID != "" {
		return ErrTrelloTask
	}
	t.NotBefore = day
	return nil
}

// SetDisplayFrequency changes how many days apart the task is displayed.
func (t *Task) SetDisplayFrequency(frequency int64) error {
	if t.TrelloID != "" {
		return ErrTrelloTask
	}
	t.DisplayFrequency = frequency
	return nil
}

// AddTag tags the task, reporting whether the tag was new.
func (t *Task) AddTag(tag string) (bool, error) {
	if t.TrelloID != "" {
		return false, ErrTrelloTask
	}
	if slices.Contains(t.Tags, tag) {
		return false, nil
	}

	t.Tags = sortedTags(append(t.Tags, tag))
	return true, nil
}

// RemoveTag removes a tag, reporting whether the task carried it.
func (t *Task) RemoveTag(tag string) (bool, error) {
	if t.TrelloID != "" {
		return false, ErrTrelloTask
	}
	index := slices.Index(t.Tags, tag)
	if index < 0 {
		return false, nil
	}

	t.Tags = slices.Delete(t.Tags, index, index+1)
	return true, nil
}

// HasTag reports whether the task carries a tag.
func (t *Task) HasTag(tag string) bool {
	return slices.Contains(t.Tags, tag)
}

// IsMature reports whether the task has reached the day it starts being
// displayed on.
func (t *Task) IsMature() bool {
	return t.NotBefore.IsZero() || !Today().Before(t.NotBefore)
}

// DisplayToday reports whether a task with a display frequency is due to be
// shown today.
func (t *Task) DisplayToday() bool {
	if t.DisplayFrequency <= 1 {
		return true
	}

	// The task ID spreads tasks that share a frequency across different
	// days, so they do not all come due at once.
	day := new(big.Int).Add(t.ID, big.NewInt(Today().DayCount()))
	remainder := new(big.Int).Mod(day, big.NewInt(t.DisplayFrequency))
	return remainder.Sign() == 0
}

// sortedTags returns the tags sorted and free of duplicates, which keeps task
// files stable from one write to the next.
func sortedTags(tags []string) []string {
	slices.Sort(tags)
	return slices.Compact(tags)
}

// splitLines splits file contents into lines, the way the Raku implementation
// read them: a single trailing newline is not a line of its own.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}
