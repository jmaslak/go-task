package task

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// newTestStore returns a store over an empty temporary directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(t.TempDir())
}

// addTask creates and saves a task with the given title.
func addTask(t *testing.T, store *Store, title string) *Task {
	t.Helper()

	number, err := store.NextNumber()
	if err != nil {
		t.Fatalf("NextNumber returned error: %v", err)
	}

	task := New(number, title)
	if err := store.Save(task); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	return task
}

// titles returns the titles of the store's tasks, in list order.
func titles(t *testing.T, store *Store) []string {
	t.Helper()

	tasks, err := store.Tasks()
	if err != nil {
		t.Fatalf("Tasks returned error: %v", err)
	}

	got := make([]string, len(tasks))
	for i, task := range tasks {
		got[i] = task.Title
	}
	return got
}

func TestEmptyStore(t *testing.T) {
	store := newTestStore(t)

	tasks, err := store.Tasks()
	if err != nil {
		t.Fatalf("Tasks returned error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("len(Tasks) = %d, want 0", len(tasks))
	}

	number, err := store.NextNumber()
	if err != nil {
		t.Fatalf("NextNumber returned error: %v", err)
	}
	if number != 1 {
		t.Errorf("NextNumber = %d, want 1", number)
	}
	if store.LockCount() != 0 {
		t.Errorf("LockCount = %d, want 0", store.LockCount())
	}
}

func TestSaveAndRead(t *testing.T) {
	store := newTestStore(t)
	addTask(t, store, "First")
	addTask(t, store, "Second")

	if want := []string{"First", "Second"}; !slices.Equal(titles(t, store), want) {
		t.Errorf("titles = %v, want %v", titles(t, store), want)
	}

	task, err := store.Task(2)
	if err != nil {
		t.Fatalf("Task returned error: %v", err)
	}
	if task.Title != "Second" || task.Number != 2 {
		t.Errorf("Task(2) = %q, number %d", task.Title, task.Number)
	}

	if _, err := store.Task(3); !errors.Is(err, ErrNotFound) {
		t.Errorf("Task(3) error = %v, want ErrNotFound", err)
	}
	if store.LockCount() != 0 {
		t.Errorf("LockCount = %d, want 0", store.LockCount())
	}
}

func TestMove(t *testing.T) {
	store := newTestStore(t)
	addTask(t, store, "First")
	addTask(t, store, "Second")
	addTask(t, store, "Third")

	if err := store.Move(3, 1); err != nil {
		t.Fatalf("Move returned error: %v", err)
	}
	if want := []string{"Third", "First", "Second"}; !slices.Equal(titles(t, store), want) {
		t.Errorf("after moving up: %v, want %v", titles(t, store), want)
	}

	if err := store.Move(1, 3); err != nil {
		t.Fatalf("Move returned error: %v", err)
	}
	if want := []string{"First", "Second", "Third"}; !slices.Equal(titles(t, store), want) {
		t.Errorf("after moving down: %v, want %v", titles(t, store), want)
	}

	// A destination past the end of the list moves the task last.
	if err := store.Move(1, 99); err != nil {
		t.Fatalf("Move returned error: %v", err)
	}
	if want := []string{"Second", "Third", "First"}; !slices.Equal(titles(t, store), want) {
		t.Errorf("after moving past the end: %v, want %v", titles(t, store), want)
	}
}

func TestCoalesce(t *testing.T) {
	store := newTestStore(t)
	addTask(t, store, "First")
	second := addTask(t, store, "Second")
	addTask(t, store, "Third")

	// Tasks removed behind the application's back leave a gap.
	if err := os.Remove(second.File); err != nil {
		t.Fatal(err)
	}

	numbers, err := store.Numbers()
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{1, 3}; !slices.Equal(numbers, want) {
		t.Errorf("numbers before coalescing = %v, want %v", numbers, want)
	}

	if err := store.Coalesce(); err != nil {
		t.Fatalf("Coalesce returned error: %v", err)
	}

	numbers, err = store.Numbers()
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{1, 2}; !slices.Equal(numbers, want) {
		t.Errorf("numbers after coalescing = %v, want %v", numbers, want)
	}
	if want := []string{"First", "Third"}; !slices.Equal(titles(t, store), want) {
		t.Errorf("titles after coalescing = %v, want %v", titles(t, store), want)
	}
}

func TestArchive(t *testing.T) {
	store := newTestStore(t)
	addTask(t, store, "First")
	addTask(t, store, "Second")

	if err := store.Archive(1); err != nil {
		t.Fatalf("Archive returned error: %v", err)
	}

	if want := []string{"Second"}; !slices.Equal(titles(t, store), want) {
		t.Errorf("titles = %v, want %v", titles(t, store), want)
	}

	// The closed task keeps its contents under the done directory.
	entries, err := os.ReadDir(filepath.Join(store.Dir(), "done"))
	if err != nil {
		t.Fatalf("reading the done directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d files under done, want 1", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(store.Dir(), "done", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	archived, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("the archived file does not parse: %v", err)
	}
	if archived.Title != "First" {
		t.Errorf("archived title = %q, want %q", archived.Title, "First")
	}
}

func TestDelete(t *testing.T) {
	store := newTestStore(t)
	addTask(t, store, "First")
	addTask(t, store, "Second")

	if err := store.Delete(1); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if want := []string{"Second"}; !slices.Equal(titles(t, store), want) {
		t.Errorf("titles = %v, want %v", titles(t, store), want)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), "done")); err == nil {
		t.Error("Delete archived the task instead of removing it")
	}
}

// A gap in the numbering is not a hole in the list: the tasks that are left
// keep their own numbers until they are coalesced.
func TestGapsInNumbering(t *testing.T) {
	store := newTestStore(t)
	first := addTask(t, store, "First")
	addTask(t, store, "Second")

	if err := os.Remove(first.File); err != nil {
		t.Fatal(err)
	}

	tasks, err := store.Tasks()
	if err != nil {
		t.Fatalf("Tasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].Title != "Second" || tasks[0].Number != 2 {
		t.Errorf("remaining task = %q, number %d", tasks[0].Title, tasks[0].Number)
	}
}

// Version 1 files gain a task ID when they are read, which has to be written
// back or the ID would change from one run to the next.
func TestVersionOneFilesAreUpgraded(t *testing.T) {
	store := newTestStore(t)
	name := filepath.Join(store.Dir(), "00001-none.task")
	if err := os.WriteFile(name, []byte("Title: Old\nCreated: 1437509667\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := store.Tasks()
	if err != nil {
		t.Fatalf("Tasks returned error: %v", err)
	}
	id := tasks[0].ID

	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	written, err := Unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if written.Version != FormatVersion {
		t.Errorf("the file on disk is still version %d", written.Version)
	}
	if written.ID.Cmp(id) != 0 {
		t.Errorf("the file on disk has ID %s, want %s", written.ID, id)
	}
}

func TestLockNests(t *testing.T) {
	store := newTestStore(t)

	err := store.WithLock(func() error {
		if store.LockCount() != 1 {
			t.Errorf("LockCount = %d, want 1", store.LockCount())
		}
		return store.WithLock(func() error {
			if store.LockCount() != 2 {
				t.Errorf("LockCount = %d, want 2", store.LockCount())
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("WithLock returned error: %v", err)
	}
	if store.LockCount() != 0 {
		t.Errorf("LockCount = %d, want 0", store.LockCount())
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("TASKDIR", "/somewhere/tasks")
	if got := DefaultDir(); got != "/somewhere/tasks" {
		t.Errorf("DefaultDir = %q, want the value of TASKDIR", got)
	}

	t.Setenv("TASKDIR", "")
	t.Setenv("HOME", "/home/someone")
	if got := DefaultDir(); got != "/home/someone/.task" {
		t.Errorf("DefaultDir = %q, want .task under the home directory", got)
	}
}
