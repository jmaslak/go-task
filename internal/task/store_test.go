package task

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
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

	// Reading one task on its own upgrades the file just the same, so that
	// two reads of the same task cannot disagree about its ID.
	other := NewStore(store.Dir())
	if err := os.WriteFile(name, []byte("Title: Old\nCreated: 1437509667\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := other.Task(1)
	if err != nil {
		t.Fatalf("Task returned error: %v", err)
	}
	second, err := other.Task(1)
	if err != nil {
		t.Fatalf("Task returned error: %v", err)
	}
	if first.ID.Cmp(second.ID) != 0 {
		t.Errorf("two reads gave IDs %s and %s", first.ID, second.ID)
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

// A lock another process holds is given up on rather than waited out forever,
// which is what left a monitor unable to answer a keystroke.
func TestLockGivesUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), lockFileName)
	held := newFileLock(path)
	// A second lock on the same file stands in for a second process: the
	// file lock goes with the open file, not with the process.
	other := newFileLock(path)

	if err := held.acquire(lockWait); err != nil {
		t.Fatalf("acquire returned error: %v", err)
	}

	start := time.Now()
	const wait = 100 * time.Millisecond
	if err := other.acquire(wait); !errors.Is(err, ErrLocked) {
		t.Errorf("acquire error = %v, want ErrLocked", err)
	}
	if waited := time.Since(start); waited < wait {
		t.Errorf("acquire gave up after %v, want it to wait %v first", waited, wait)
	}

	// The lock this one already holds is taken again without waiting at all.
	if err := held.acquire(0); err != nil {
		t.Errorf("nested acquire returned error: %v", err)
	}
	if held.held() != 2 {
		t.Errorf("held = %d, want 2", held.held())
	}
	for range 2 {
		if err := held.release(); err != nil {
			t.Fatalf("release returned error: %v", err)
		}
	}

	// With the lock given up, the other one can have it.
	if err := other.acquire(wait); err != nil {
		t.Errorf("acquire returned error once the lock was free: %v", err)
	}
	if err := other.release(); err != nil {
		t.Errorf("release returned error: %v", err)
	}
}

func TestTryWithLock(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	other := NewStore(dir)

	err := store.WithLock(func() error {
		ran := false
		err := other.TryWithLock(func() error {
			ran = true
			return nil
		})
		if !errors.Is(err, ErrLocked) {
			t.Errorf("TryWithLock error = %v, want ErrLocked", err)
		}
		if ran {
			t.Error("TryWithLock ran its function while another store held the lock")
		}

		// A lock this store already holds nests, as it does for WithLock.
		return store.TryWithLock(func() error { return nil })
	})
	if err != nil {
		t.Fatalf("WithLock returned error: %v", err)
	}
	if store.LockCount() != 0 {
		t.Errorf("LockCount = %d, want 0", store.LockCount())
	}
}

// Nothing read without the lock can be trusted afterwards, so a freshly taken
// lock starts with nothing cached.
func TestFreshLockDropsTheCache(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	other := NewStore(dir)

	addTask(t, store, "First")
	if got := titles(t, store); !slices.Equal(got, []string{"First"}) {
		t.Fatalf("titles = %v", got)
	}

	// Another process adds a task while this store holds no lock.
	addTask(t, other, "Second")

	if got := titles(t, store); !slices.Equal(got, []string{"First", "Second"}) {
		t.Errorf("titles = %v, want both tasks", got)
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
