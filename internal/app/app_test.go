package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jmaslak/go-task/config"
	"github.com/jmaslak/go-task/task"
)

// newTestApp returns an application over an empty task directory, reading the
// given input and writing everything it produces into the returned buffer.
func newTestApp(t *testing.T, input string) (*App, *bytes.Buffer) {
	t.Helper()

	cfg := config.NoColor()
	// Show output where the test can see it rather than in a pager, and
	// keep the editor from being reached by accident.
	cfg.PagerCommand = ""
	cfg.EditorCommand = ""

	out := &bytes.Buffer{}
	return &App{
		Store:  task.NewStore(t.TempDir()),
		Config: cfg,
		In:     bufio.NewReader(strings.NewReader(input)),
		Out:    out,
		Err:    out,
	}, out
}

// mustNewTask creates a task with a title and fails the test if it cannot.
func mustNewTask(t *testing.T, a *App, title string) int {
	t.Helper()

	number, err := a.NewTask(title, NewOptions{})
	if err != nil {
		t.Fatalf("NewTask(%q) returned error: %v", title, err)
	}
	return number
}

// mustTasks returns the application's tasks.
func mustTasks(t *testing.T, a *App) []*task.Task {
	t.Helper()

	tasks, err := a.Store.Tasks()
	if err != nil {
		t.Fatalf("Tasks returned error: %v", err)
	}
	return tasks
}

// noteTexts returns the text of each note on a task.
func noteTexts(tasks []*task.Task, index int) []string {
	texts := make([]string, len(tasks[index].Notes))
	for i, note := range tasks[index].Notes {
		texts[i] = note.Text
	}
	return texts
}

func TestNewTask(t *testing.T) {
	a, _ := newTestApp(t, "")

	if number := mustNewTask(t, a, "  Subject\tLine  "); number != 1 {
		t.Errorf("NewTask = %d, want 1", number)
	}

	tasks := mustTasks(t, a)
	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
	// A title is trimmed, and its tabs flattened, so it stays one line of
	// the task file.
	if tasks[0].Title != "Subject Line" {
		t.Errorf("Title = %q, want %q", tasks[0].Title, "Subject Line")
	}
	if len(tasks[0].Notes) != 0 {
		t.Errorf("len(Notes) = %d, want 0", len(tasks[0].Notes))
	}
	if len(tasks[0].Tags) != 0 {
		t.Errorf("len(Tags) = %d, want 0", len(tasks[0].Tags))
	}

	if number := mustNewTask(t, a, "Second"); number != 2 {
		t.Errorf("NewTask = %d, want 2", number)
	}
	if a.Store.LockCount() != 0 {
		t.Errorf("LockCount = %d, want 0", a.Store.LockCount())
	}
}

// Without a title on the command line, the title is asked for, then a note,
// then whether to save at all.
func TestNewTaskInteractive(t *testing.T) {
	a, _ := newTestApp(t, "Subject Line\nn\n\n")

	if number, err := a.NewTask("", NewOptions{}); err != nil || number != 1 {
		t.Fatalf("NewTask = %d, %v", number, err)
	}

	tasks := mustTasks(t, a)
	if len(tasks) != 1 || tasks[0].Title != "Subject Line" {
		t.Fatalf("tasks = %v", tasks)
	}
	if len(tasks[0].Notes) != 0 {
		t.Errorf("len(Notes) = %d, want 0", len(tasks[0].Notes))
	}
}

func TestNewTaskAborted(t *testing.T) {
	a, out := newTestApp(t, "Subject Line\nn\nn\n")

	if _, err := a.NewTask("", NewOptions{}); !errors.Is(err, ErrAborted) {
		t.Fatalf("NewTask error = %v, want ErrAborted", err)
	}
	if !strings.Contains(out.String(), "Aborting.") {
		t.Errorf("output did not say it was aborting: %q", out)
	}
	if tasks := mustTasks(t, a); len(tasks) != 0 {
		t.Errorf("%d tasks were created despite aborting", len(tasks))
	}
}

func TestNewTaskWithOptions(t *testing.T) {
	a, _ := newTestApp(t, "")

	maturity := task.Today().AddDays(7)
	if _, err := a.NewTask("Later", NewOptions{MaturityDate: maturity, Tag: "someday"}); err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}
	if _, err := a.NewTask("Today only", NewOptions{ExpireToday: true}); err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if tasks[0].NotBefore != maturity {
		t.Errorf("NotBefore = %v, want %v", tasks[0].NotBefore, maturity)
	}
	if want := []string{"someday"}; !slices.Equal(tasks[0].Tags, want) {
		t.Errorf("Tags = %v, want %v", tasks[0].Tags, want)
	}
	if tasks[1].Expires != task.Today() {
		t.Errorf("Expires = %v, want today", tasks[1].Expires)
	}

	// The two options describe different lives for a task; asking for both
	// is a mistake worth reporting.
	_, err := a.NewTask("Both", NewOptions{ExpireToday: true, MaturityDate: maturity})
	if err == nil {
		t.Error("NewTask accepted both --expire-today and --maturity-date")
	}
}

func TestAddNote(t *testing.T) {
	a, out := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")

	if err := a.AddNote(1, "A Note."); err != nil {
		t.Fatalf("AddNote returned error: %v", err)
	}
	if err := a.AddNote(1, "B Note."); err != nil {
		t.Fatalf("AddNote returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if want := []string{"A Note.", "B Note."}; !slices.Equal(noteTexts(tasks, 0), want) {
		t.Errorf("notes = %v, want %v", noteTexts(tasks, 0), want)
	}
	if tasks[0].Notes[0].Date.IsZero() {
		t.Error("the note has no date")
	}
	if !strings.Contains(out.String(), "Updated task 1") {
		t.Errorf("output did not report the update: %q", out)
	}

	if err := a.AddNote(9, "Nowhere."); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("AddNote to a missing task = %v, want ErrNotFound", err)
	}
	if !strings.Contains(out.String(), "Could not locate task number 9") {
		t.Errorf("output did not report the missing task: %q", out)
	}
}

func TestRetitle(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")

	if err := a.Retitle(1, "New Subject Line"); err != nil {
		t.Fatalf("Retitle returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if tasks[0].Title != "New Subject Line" {
		t.Errorf("Title = %q", tasks[0].Title)
	}

	// The old title is not lost; the change is recorded as a note.
	want := []string{"Title changed from:\n  Subject Line\nTo:\n  New Subject Line"}
	if !slices.Equal(noteTexts(tasks, 0), want) {
		t.Errorf("notes = %q, want %q", noteTexts(tasks, 0), want)
	}
}

func TestTagCommands(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")

	steps := []struct {
		add   bool
		tag   string
		tags  []string
		notes []string
	}{
		{true, "test1", []string{"test1"}, []string{"Added tag test1"}},
		// Adding a tag twice changes nothing, and records nothing.
		{true, "test1", []string{"test1"}, []string{"Added tag test1"}},
		{true, "test2", []string{"test1", "test2"}, []string{"Added tag test1", "Added tag test2"}},
		{false, "bogus", []string{"test1", "test2"}, []string{"Added tag test1", "Added tag test2"}},
		{false, "test1", []string{"test2"}, []string{"Added tag test1", "Added tag test2", "Removed tag test1"}},
		{false, "test2", nil, []string{"Added tag test1", "Added tag test2", "Removed tag test1", "Removed tag test2"}},
	}

	for i, step := range steps {
		var err error
		if step.add {
			err = a.AddTag(1, step.tag)
		} else {
			err = a.RemoveTag(1, step.tag)
		}
		if err != nil {
			t.Fatalf("step %d returned error: %v", i, err)
		}

		tasks := mustTasks(t, a)
		if !slices.Equal(tasks[0].Tags, step.tags) {
			t.Errorf("step %d: tags = %v, want %v", i, tasks[0].Tags, step.tags)
		}
		if !slices.Equal(noteTexts(tasks, 0), step.notes) {
			t.Errorf("step %d: notes = %v, want %v", i, noteTexts(tasks, 0), step.notes)
		}
	}

	if a.Store.LockCount() != 0 {
		t.Errorf("LockCount = %d, want 0", a.Store.LockCount())
	}
}

func TestSetExpiration(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")

	today := task.Today()
	if err := a.SetExpiration(1, today); err != nil {
		t.Fatalf("SetExpiration returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if tasks[0].Expires != today {
		t.Errorf("Expires = %v, want %v", tasks[0].Expires, today)
	}
	if want := []string{"Added expiration date: " + today.String()}; !slices.Equal(noteTexts(tasks, 0), want) {
		t.Errorf("notes = %v, want %v", noteTexts(tasks, 0), want)
	}

	tomorrow := today.AddDays(1)
	if err := a.SetExpiration(1, tomorrow); err != nil {
		t.Fatalf("SetExpiration returned error: %v", err)
	}

	tasks = mustTasks(t, a)
	if tasks[0].Expires != tomorrow {
		t.Errorf("Expires = %v, want %v", tasks[0].Expires, tomorrow)
	}
	want := "Updated expiration date from " + today.String() + " to " + tomorrow.String()
	if got := noteTexts(tasks, 0); len(got) != 2 || got[1] != want {
		t.Errorf("notes = %v, want the second to be %q", got, want)
	}

	// An expiration date in the past would close the task on the spot.
	if err := a.SetExpiration(1, today.AddDays(-1)); err == nil {
		t.Error("SetExpiration accepted a date before today")
	}
}

func TestExpire(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "Still relevant")
	mustNewTask(t, a, "Past it")

	// Reach past the command, which refuses dates in the past, to set up a
	// task that has already expired.
	past, err := a.Store.Task(2)
	if err != nil {
		t.Fatal(err)
	}
	if err := past.SetExpires(task.Today().AddDays(-1)); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Save(past); err != nil {
		t.Fatal(err)
	}

	if err := a.Expire(); err != nil {
		t.Fatalf("Expire returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if len(tasks) != 1 || tasks[0].Title != "Still relevant" {
		t.Fatalf("tasks after expiring = %v", tasks)
	}
	// Closing renumbers what is left.
	if tasks[0].Number != 1 {
		t.Errorf("remaining task is numbered %d, want 1", tasks[0].Number)
	}

	// A task that expires today is not expired yet.
	mustNewTask(t, a, "Today only")
	if err := a.SetExpiration(2, task.Today()); err != nil {
		t.Fatal(err)
	}
	if err := a.Expire(); err != nil {
		t.Fatalf("Expire returned error: %v", err)
	}
	if len(mustTasks(t, a)) != 2 {
		t.Error("a task expiring today was closed early")
	}
}

func TestSetMaturity(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")

	tomorrow := task.Today().AddDays(1)
	if err := a.SetMaturity(1, tomorrow); err != nil {
		t.Fatalf("SetMaturity returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if tasks[0].NotBefore != tomorrow {
		t.Errorf("NotBefore = %v, want %v", tasks[0].NotBefore, tomorrow)
	}
	if want := []string{"Added not-before date: " + tomorrow.String()}; !slices.Equal(noteTexts(tasks, 0), want) {
		t.Errorf("notes = %v, want %v", noteTexts(tasks, 0), want)
	}

	later := tomorrow.AddDays(1)
	if err := a.SetMaturity(1, later); err != nil {
		t.Fatalf("SetMaturity returned error: %v", err)
	}

	want := "Updated not-before date from " + tomorrow.String() + " to " + later.String()
	if got := noteTexts(mustTasks(t, a), 0); len(got) != 2 || got[1] != want {
		t.Errorf("notes = %v, want the second to be %q", got, want)
	}

	// A task that matures today or earlier is simply a task.
	if err := a.SetMaturity(1, task.Today()); err == nil {
		t.Error("SetMaturity accepted today")
	}
}

func TestSetFrequency(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")

	if err := a.SetFrequency(1, 1); err != nil {
		t.Fatalf("SetFrequency returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if tasks[0].DisplayFrequency != 1 {
		t.Errorf("DisplayFrequency = %d, want 1", tasks[0].DisplayFrequency)
	}
	if !tasks[0].DisplayToday() {
		t.Error("a task displayed every day is being held back")
	}
	if want := []string{"Added display frequency of every 1 day"}; !slices.Equal(noteTexts(tasks, 0), want) {
		t.Errorf("notes = %v, want %v", noteTexts(tasks, 0), want)
	}

	if err := a.SetFrequency(1, 1_000_000_000_000); err != nil {
		t.Fatalf("SetFrequency returned error: %v", err)
	}

	tasks = mustTasks(t, a)
	if tasks[0].DisplayFrequency != 1_000_000_000_000 {
		t.Errorf("DisplayFrequency = %d", tasks[0].DisplayFrequency)
	}
	if tasks[0].DisplayToday() {
		t.Error("a task with a huge display frequency came due today")
	}
	want := "Updated display frequency from every 1 day to every 1000000000000 days"
	if got := noteTexts(tasks, 0); len(got) != 2 || got[1] != want {
		t.Errorf("notes = %v, want the second to be %q", got, want)
	}
}

func TestMove(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")
	mustNewTask(t, a, "Second Task")

	if err := a.Move(2, 1); err != nil {
		t.Fatalf("Move returned error: %v", err)
	}
	tasks := mustTasks(t, a)
	if tasks[0].Title != "Second Task" || tasks[1].Title != "Subject Line" {
		t.Fatalf("after moving up: %q, %q", tasks[0].Title, tasks[1].Title)
	}

	if err := a.Move(1, 2); err != nil {
		t.Fatalf("Move returned error: %v", err)
	}
	tasks = mustTasks(t, a)
	if tasks[0].Title != "Subject Line" || tasks[1].Title != "Second Task" {
		t.Fatalf("after moving down: %q, %q", tasks[0].Title, tasks[1].Title)
	}
}

func TestCloseTask(t *testing.T) {
	a, out := newTestApp(t, "n\n")
	mustNewTask(t, a, "First")
	mustNewTask(t, a, "Second")

	if err := a.CloseTask(1); err != nil {
		t.Fatalf("CloseTask returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if len(tasks) != 1 || tasks[0].Title != "Second" || tasks[0].Number != 1 {
		t.Fatalf("tasks after closing = %v", tasks)
	}
	if !strings.Contains(out.String(), "Closed 00001") {
		t.Errorf("output did not report the close: %q", out)
	}

	entries, err := os.ReadDir(filepath.Join(a.Store.Dir(), "done"))
	if err != nil || len(entries) != 1 {
		t.Errorf("done directory holds %d files (%v)", len(entries), err)
	}
}

func TestCoalesce(t *testing.T) {
	a, out := newTestApp(t, "")
	mustNewTask(t, a, "First")
	second := mustNewTask(t, a, "Second")
	mustNewTask(t, a, "Third")

	tasks := mustTasks(t, a)
	if err := os.Remove(tasks[second-1].File); err != nil {
		t.Fatal(err)
	}

	if err := a.Coalesce(); err != nil {
		t.Fatalf("Coalesce returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Coalesced tasks") {
		t.Errorf("output did not report the coalesce: %q", out)
	}

	tasks = mustTasks(t, a)
	if len(tasks) != 2 || tasks[0].Number != 1 || tasks[1].Number != 2 {
		t.Errorf("tasks after coalescing = %v", tasks)
	}
}

func TestTaskListFiltering(t *testing.T) {
	a, _ := newTestApp(t, "")
	a.Config.IgnoreTags = []string{"someday"}

	mustNewTask(t, a, "Plain")
	mustNewTask(t, a, "Immature")
	mustNewTask(t, a, "Ignored")
	mustNewTask(t, a, "Tagged")

	if err := a.SetMaturity(2, task.Today().AddDays(1)); err != nil {
		t.Fatal(err)
	}
	if err := a.AddTag(3, "someday"); err != nil {
		t.Fatal(err)
	}
	if err := a.AddTag(4, "work"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		opts  ListOptions
		want  []string
		avoid []string
	}{
		{"default", ListOptions{}, []string{"Plain", "Tagged"}, []string{"Immature", "Ignored"}},
		{"show immature", ListOptions{ShowImmature: true}, []string{"Plain", "Immature"}, []string{"Ignored"}},
		{"all", ListOptions{All: true}, []string{"Plain", "Immature", "Ignored", "Tagged"}, nil},
		{"by tag", ListOptions{Tag: "someday"}, []string{"Ignored"}, []string{"Plain", "Tagged"}},
		{"limited", ListOptions{Max: 1}, []string{"Plain"}, []string{"Tagged"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := a.taskList(tt.opts)
			if err != nil {
				t.Fatalf("taskList returned error: %v", err)
			}
			for _, title := range tt.want {
				if !strings.Contains(list, title) {
					t.Errorf("listing is missing %q:\n%s", title, list)
				}
			}
			for _, title := range tt.avoid {
				if strings.Contains(list, title) {
					t.Errorf("listing should not hold %q:\n%s", title, list)
				}
			}
		})
	}
}

func TestTaskListTruncatesToWidth(t *testing.T) {
	a, _ := newTestApp(t, "")
	mustNewTask(t, a, strings.Repeat("x", 100))
	if err := a.AddTag(1, "tag"); err != nil {
		t.Fatal(err)
	}

	list, err := a.taskList(ListOptions{Width: 20})
	if err != nil {
		t.Fatalf("taskList returned error: %v", err)
	}

	line := strings.TrimSuffix(list, "\n")
	if len([]rune(line)) > 20 {
		t.Errorf("line is %d characters wide, want at most 20: %q", len([]rune(line)), line)
	}
}

func TestShow(t *testing.T) {
	a, out := newTestApp(t, "")
	mustNewTask(t, a, "Subject Line")
	if err := a.AddNote(1, "A note."); err != nil {
		t.Fatal(err)
	}
	if err := a.AddTag(1, "work"); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := a.Show(1); err != nil {
		t.Fatalf("Show returned error: %v", err)
	}

	shown := out.String()
	for _, want := range []string{"Title", "Subject Line", "Created", "Task-ID", "Tags", "work", "A note."} {
		if !strings.Contains(shown, want) {
			t.Errorf("output is missing %q:\n%s", want, shown)
		}
	}
}

func TestYesNoPrompt(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"N\n", false},
		{"n\n", false},
		{"no\n", false},
		{"NO\n", false},
		{"Y\n", true},
		{"y\n", true},
		{"yes\n", true},
		{"\n", true},
		// An answer that is neither is asked again.
		{"yeppers\ny\n", true},
		// Input that runs out is a no.
		{"", false},
	}

	for _, tt := range tests {
		a, _ := newTestApp(t, tt.input)
		a.Quiet = true
		if got := a.yesNoPrompt(""); got != tt.want {
			t.Errorf("yesNoPrompt(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// Commands that take a task number refuse to run when the numbers may have
// shifted since the terminal last saw them.
func TestStaleTaskList(t *testing.T) {
	a, out := newTestApp(t, "")
	a.Interactive = true
	a.CheckFreshness = true

	mustNewTask(t, a, "First")

	if err := a.Retitle(1, "Renamed"); !errors.Is(err, ErrStaleTaskList) {
		t.Fatalf("Retitle error = %v, want ErrStaleTaskList", err)
	}
	if !strings.Contains(out.String(), "task numbers may have changed") {
		t.Errorf("output did not explain the refusal: %q", out)
	}

	// Listing the tasks records what this terminal has been shown.
	if err := a.List(ListOptions{}); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if err := a.Retitle(1, "Renamed"); err != nil {
		t.Fatalf("Retitle returned error after a listing: %v", err)
	}

	// Renaming the task changed the listing, so the record is stale again.
	if err := a.Retitle(1, "Renamed twice"); !errors.Is(err, ErrStaleTaskList) {
		t.Errorf("Retitle error = %v, want ErrStaleTaskList", err)
	}
}

func TestNoteFromEditor(t *testing.T) {
	a, _ := newTestApp(t, "y\n\n")
	mustNewTask(t, a, "Subject Line")

	// An editor that appends a note, and leaves the header in place.
	editor := filepath.Join(t.TempDir(), "editor")
	script := "#!/bin/sh\nprintf '\\nWhat the user typed.\\n\\n' >> \"$1\"\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	a.Config.EditorCommand = editor + " %FILENAME%"

	if err := a.AddNote(1, ""); err != nil {
		t.Fatalf("AddNote returned error: %v", err)
	}

	tasks := mustTasks(t, a)
	if want := []string{"What the user typed."}; !slices.Equal(noteTexts(tasks, 0), want) {
		t.Errorf("notes = %v, want %v", noteTexts(tasks, 0), want)
	}
}

// writeScript writes an executable shell script and returns the editor command
// that runs it.
func writeScript(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "script")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path + " %FILENAME%"
}

// waitForFile waits for a file another process is expected to create.
func waitForFile(t *testing.T, path string) {
	t.Helper()

	for range 200 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s was never created", path)
}

// The editor takes as long as its user does, so the task directory must not be
// locked while it runs: it used to be, which left every other task process -
// a monitor included - waiting on the lock for as long as the editor was open.
func TestEditorRunsWithoutTheLock(t *testing.T) {
	a, _ := newTestApp(t, "y\n\n")
	mustNewTask(t, a, "Subject Line")

	signals := t.TempDir()
	started := filepath.Join(signals, "started")
	resume := filepath.Join(signals, "resume")
	a.Config.EditorCommand = writeScript(t, fmt.Sprintf(
		"printf '\\nWhat the user typed.\\n' >> \"$1\"\n"+
			"touch %s\n"+
			"until [ -f %s ]; do sleep 0.05; done\n", started, resume))

	done := make(chan error, 1)
	go func() { done <- a.AddNote(1, "") }()

	waitForFile(t, started)
	other := task.NewStore(a.Store.Dir())
	if err := other.TryWithLock(func() error { return nil }); err != nil {
		t.Errorf("the task directory was locked while the editor was open: %v", err)
	}
	if err := os.WriteFile(resume, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := <-done; err != nil {
		t.Fatalf("AddNote returned error: %v", err)
	}
	if want := []string{"What the user typed."}; !slices.Equal(noteTexts(mustTasks(t, a), 0), want) {
		t.Errorf("notes = %v, want %v", noteTexts(mustTasks(t, a), 0), want)
	}
}

// With the lock released while the editor is open, another process can renumber
// the tasks underneath it. The note must not land on whatever task ends up
// wearing the number it was written for.
func TestNoteRefusedWhenTheTaskChanged(t *testing.T) {
	a, out := newTestApp(t, "y\n\n")
	mustNewTask(t, a, "First")
	mustNewTask(t, a, "Second")

	// An editor that swaps the two tasks over, as another process would.
	dir := a.Store.Dir()
	a.Config.EditorCommand = writeScript(t, fmt.Sprintf(
		"printf '\\nA note.\\n' >> \"$1\"\n"+
			"cd %s\n"+
			"mv 00001-none.task swapping\n"+
			"mv 00002-none.task 00001-none.task\n"+
			"mv swapping 00002-none.task\n", dir))

	err := a.AddNote(1, "")
	if err == nil || !strings.Contains(err.Error(), "no longer the task that was shown") {
		t.Fatalf("AddNote error = %v, want a refusal naming the changed task", err)
	}
	if strings.Contains(out.String(), "Updated task") {
		t.Errorf("output claimed the task was updated: %q", out)
	}

	for i, tasks := 0, mustTasks(t, a); i < len(tasks); i++ {
		if len(tasks[i].Notes) != 0 {
			t.Errorf("task %d gained a note", tasks[i].Number)
		}
	}
}

func TestTrimNote(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
	}{
		{
			"header and padding are stripped",
			editorPrompt + "\n" + editorRule + "\n\n\nThe note.\n\n",
			"The note.",
		},
		{
			"an untouched file leaves nothing",
			editorPrompt + "\n" + editorRule + "\n",
			"",
		},
		{
			"blank lines inside the note are kept",
			editorPrompt + "\n" + editorRule + "\nOne.\n\nTwo.\n",
			"One.\n\nTwo.",
		},
		{
			"a file with no header at all still works",
			"Just this.\n",
			"Just this.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trimNote(tt.contents); got != tt.want {
				t.Errorf("trimNote = %q, want %q", got, tt.want)
			}
		})
	}
}
