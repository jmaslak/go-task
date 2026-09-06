package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode/utf8"

	"github.com/jmaslak/go-task/internal/task"
)

// ctimeLayout renders a timestamp the way the C library's ctime does, which
// is the format task files have always displayed.
const ctimeLayout = "Mon Jan _2 15:04:05 2006"

// headerNames are the display names of the task headers, in the order they
// are shown.
var headerNames = []struct {
	key  string
	name string
}{
	{"title", "Title"},
	{"created", "Created"},
	{"not-before", "Not-Before"},
	{"expires", "Expires"},
	{"display-frequency", "Display-Frequency"},
	{"task-id", "Task-ID"},
	{"tags", "Tags"},
}

// headerWidth is the column the header values line up at.
var headerWidth = func() int {
	width := 0
	for _, header := range headerNames {
		width = max(width, len(header.name))
	}
	return width
}()

// ListOptions selects which tasks a listing shows.
type ListOptions struct {
	// Max caps the number of tasks listed; zero lists them all.
	Max int
	// Width truncates each line to fit a terminal that wide; zero leaves
	// the lines untruncated.
	Width int
	// ShowImmature includes tasks that have not reached their maturity
	// date.
	ShowImmature bool
	// All includes tasks held back by a display frequency or an ignored
	// tag, and implies ShowImmature.
	All bool
	// Tag, when set, limits the listing to tasks carrying that tag.
	Tag string
}

// selected returns the tasks the options ask for, in list order.
func (a *App) selected(opts ListOptions) ([]*task.Task, error) {
	tasks, err := a.Store.Tasks()
	if err != nil {
		return nil, err
	}

	showImmature := opts.ShowImmature || opts.All

	var selected []*task.Task
	for _, t := range tasks {
		switch {
		case opts.Tag != "" && !t.HasTag(opts.Tag):
			// Not the tag being asked for.
		case opts.All:
			selected = append(selected, t)
		case opts.Tag == "" && a.Config.HasIgnoredTag(t.Tags):
			// Ignored tags are shown only when asked for by name.
		case !t.DisplayToday():
			// Not due to be displayed today.
		case showImmature || t.IsMature():
			selected = append(selected, t)
		}
	}

	if opts.Max > 0 && len(selected) > opts.Max {
		selected = selected[:opts.Max]
	}

	return selected, nil
}

// taskList renders the task listing.
func (a *App) taskList(opts ListOptions) (string, error) {
	tasks, err := a.selected(opts)
	if err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return "", nil
	}

	widest := 0
	for _, t := range tasks {
		widest = max(widest, len(fmt.Sprint(t.Number)))
	}

	var b strings.Builder
	for _, t := range tasks {
		tags := ""
		if len(t.Tags) > 0 {
			tags = "[" + strings.Join(t.Tags, "] [") + "] "
		}

		title := t.Title
		if opts.Width > 0 {
			// The number, the space after it, and the tags all take room
			// away from the title.
			title = truncate(title, opts.Width-widest-1-utf8.RuneCountInString(tags))
		}

		color := a.Config.PromptBoldColor
		switch {
		case !t.IsMature():
			color = a.Config.ImmatureTaskColor
		case !t.DisplayToday():
			color = a.Config.NotDisplayedTodayColor
		}

		fmt.Fprintf(&b, "%s%d %s%s%s%s%s\n",
			a.Config.PromptInfoColor, t.Number,
			a.Config.TagColor, tags,
			color, title,
			a.Config.Reset)
	}

	return b.String(), nil
}

// show renders a single task, headers first and then its notes.
func (a *App) show(t *task.Task) string {
	var b strings.Builder

	b.WriteString(a.headerLine("title", t.Title, false))
	b.WriteString(a.headerLine("created", t.Created.Format(ctimeLayout), false))
	if !t.NotBefore.IsZero() {
		b.WriteString(a.dateHeaderLine("not-before", t.NotBefore))
	}
	if !t.Expires.IsZero() {
		b.WriteString(a.dateHeaderLine("expires", t.Expires))
	}
	if t.DisplayFrequency != 0 {
		b.WriteString(a.headerLine("display-frequency", fmt.Sprint(t.DisplayFrequency), false))
	}
	b.WriteString(a.headerLine("task-id", t.ID.Text(16), false))
	if len(t.Tags) > 0 {
		b.WriteString(a.headerLine("tags", strings.Join(t.Tags, " "), false))
	}

	b.WriteString("\n")

	notes := make([]string, len(t.Notes))
	for i, note := range t.Notes {
		notes[i] = a.note(note)
	}
	if len(notes) > 0 {
		b.WriteString(strings.Join(notes, "\n\n"))
		b.WriteString("\n")
	}

	return b.String()
}

// headerLine renders one "Name : value" line of a task's headers.
func (a *App) headerLine(key, value string, alert bool) string {
	name := key
	for _, header := range headerNames {
		if header.key == key {
			name = header.name
		}
	}

	color := a.Config.HeaderNormalColor
	if alert {
		color = a.Config.HeaderAlertColor
	}

	return fmt.Sprintf("%s%-*s : %s%s%s\n",
		a.Config.HeaderTitleColor, headerWidth, name, color, value, a.Config.Reset)
}

// dateHeaderLine renders a header holding a date, calling out dates that are
// not today.
func (a *App) dateHeaderLine(key string, day task.Date) string {
	today := task.Today()
	switch {
	case day.Before(today):
		return a.headerLine(key, day.Pretty()+" (expired)", true)
	case day.After(today):
		return a.headerLine(key, day.Pretty()+" (future)", true)
	default:
		return a.headerLine(key, day.Pretty(), false)
	}
}

// note renders a single dated note of a task.
func (a *App) note(n task.Note) string {
	header := fmt.Sprintf("%s[%s]%s:%s\n",
		a.Config.HeaderAlertColor, n.Date.Format(ctimeLayout),
		a.Config.HeaderSeperatorColor, a.Config.Reset)

	body := a.Config.BodyColor + strings.ReplaceAll(n.Text, "\n", "\n"+a.Config.BodyColor)

	return header + body + a.Config.Reset
}

// displayWithPager shows text through the configured pager. If the pager is
// not installed the text is written straight out instead.
func (a *App) displayWithPager(description, contents string) error {
	temp, err := os.CreateTemp("", "task-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if _, err := temp.WriteString(contents); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	prompt := description + " ( press h for help or q to quit ) "
	argv := expandCommand(a.Config.PagerCommand, map[string]string{
		"%FILENAME%": temp.Name(),
		"%PROMPT%":   prompt,
	})
	if len(argv) == 0 {
		// An empty pager command asks for no paging at all.
		a.printf("%s", contents)
		return nil
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	if err := cmd.Run(); err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			return err
		}
		// No pager on this system; show the text as it is.
		a.printf("%s", contents)
	}

	return nil
}

// expandCommand splits a configured command line into arguments and
// substitutes the placeholders. Splitting before substituting means a
// replacement holding spaces stays a single argument.
func expandCommand(command string, replacements map[string]string) []string {
	argv := strings.Fields(command)
	for i, arg := range argv {
		for placeholder, value := range replacements {
			arg = strings.ReplaceAll(arg, placeholder, value)
		}
		argv[i] = arg
	}
	return argv
}

// truncate shortens a string to at most n characters, counting characters
// rather than bytes.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// clearScreen moves the cursor home and erases the display.
func (a *App) clearScreen() {
	a.printf("\x1b[2J\x1b[;H")
}
