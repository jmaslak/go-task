package task

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxNumber is one past the highest task number the file names can hold.
const MaxNumber = 100_000

// doneDir is the subdirectory of the task directory holding closed tasks.
const doneDir = "done"

// lockFileName is the file the directory lock is taken on.
const lockFileName = ".taskview.lock"

// taskFile matches the name of an open task file, capturing its number and
// the rest of the name, which is carried along when the task is renumbered.
var taskFile = regexp.MustCompile(`^(\d+)(-.*\.task)$`)

// ErrNotFound is returned for a task number that is not in the directory.
var ErrNotFound = errors.New("task not found")

// Store is the directory of task files.
type Store struct {
	dir  string
	lock *fileLock

	mu     sync.Mutex
	cached []*Task
}

// DefaultDir returns the task directory: $TASKDIR if it is set, otherwise
// .task under the user's home directory, otherwise .task under the working
// directory.
func DefaultDir() string {
	if dir := os.Getenv("TASKDIR"); dir != "" {
		return dir
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".task")
	}
	return ".task"
}

// NewStore returns the store for a task directory. The directory is created
// on first use if it does not already exist.
func NewStore(dir string) *Store {
	return &Store{
		dir:  dir,
		lock: newFileLock(filepath.Join(dir, lockFileName)),
	}
}

// Dir returns the task directory.
func (s *Store) Dir() string { return s.dir }

// LockCount reports how many nested locks are currently held.
func (s *Store) LockCount() int { return s.lock.held() }

// WithLock runs f while holding the task directory lock, which keeps other
// processes running the application out of the directory. Calls nest.
func (s *Store) WithLock(f func() error) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("could not create task directory: %w", err)
	}
	if err := s.lock.acquire(); err != nil {
		return err
	}
	defer func() {
		if err := s.lock.release(); err != nil {
			panic(err)
		}
	}()

	return f()
}

// Filenames returns the paths of the open task files, ordered by task number.
func (s *Store) Filenames() ([]string, error) {
	var names []string
	err := s.WithLock(func() error {
		entries, err := os.ReadDir(s.dir)
		if err != nil {
			return fmt.Errorf("could not read task directory: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && taskFile.MatchString(entry.Name()) {
				names = append(names, filepath.Join(s.dir, entry.Name()))
			}
		}
		slices.Sort(names)
		return nil
	})

	return names, err
}

// Numbers returns the numbers of the open tasks, in order.
func (s *Store) Numbers() ([]int, error) {
	names, err := s.Filenames()
	if err != nil {
		return nil, err
	}

	numbers := make([]int, 0, len(names))
	for _, name := range names {
		number, _, ok := parseName(filepath.Base(name))
		if !ok {
			continue
		}
		numbers = append(numbers, number)
	}
	slices.Sort(numbers)

	return numbers, nil
}

// Tasks returns every open task, ordered by task number. The result is cached
// until the store is changed.
func (s *Store) Tasks() ([]*Task, error) {
	s.mu.Lock()
	cached := s.cached
	s.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	var tasks []*Task
	err := s.WithLock(func() error {
		names, err := s.Filenames()
		if err != nil {
			return err
		}
		for _, name := range names {
			t, err := readTask(name)
			if err != nil {
				return err
			}
			tasks = append(tasks, t)
		}
		slices.SortFunc(tasks, func(a, b *Task) int { return cmp.Compare(a.Number, b.Number) })

		// Version 1 files gained an ID when they were read; write them
		// back out so the ID sticks.
		for _, t := range tasks {
			if t.Version < FormatVersion {
				t.Version = FormatVersion
				if err := s.write(t); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cached = tasks
	s.mu.Unlock()

	return tasks, nil
}

// Task returns a single open task by number.
func (s *Store) Task(number int) (*Task, error) {
	var t *Task
	err := s.WithLock(func() error {
		name, err := s.filename(number)
		if err != nil {
			return err
		}
		t, err = readTask(name)
		return err
	})

	return t, err
}

// NextNumber returns the number a newly created task should get.
func (s *Store) NextNumber() (int, error) {
	numbers, err := s.Numbers()
	if err != nil {
		return 0, err
	}
	if len(numbers) == 0 {
		return 1, nil
	}

	next := slices.Max(numbers) + 1
	if next >= MaxNumber {
		return 0, errors.New("task number would be too large")
	}
	return next, nil
}

// Save writes a task to disk, creating its file if it does not have one yet.
func (s *Store) Save(t *Task) error {
	return s.WithLock(func() error { return s.write(t) })
}

// Move renumbers a task, shifting the tasks between its old and new positions
// to make room. A destination past the end of the list moves the task last.
func (s *Store) Move(from, to int) error {
	return s.WithLock(func() error {
		next, err := s.NextNumber()
		if err != nil {
			return err
		}
		to = min(max(to, 1), next-1)
		if from == to {
			return nil
		}

		name, err := s.filename(from)
		if err != nil {
			return err
		}
		_, suffix, _ := parseName(filepath.Base(name))

		// Park the task under a name no listing will pick up, so that
		// renumbering the tasks it passes cannot collide with it.
		parked := filepath.Join(s.dir, fmt.Sprintf(".moving-%05d%s", to, suffix))
		if err := os.Rename(name, parked); err != nil {
			return err
		}
		s.invalidate()

		numbers, err := s.Numbers()
		if err != nil {
			return err
		}
		if to < from {
			// Shift upwards from the top, so each task moves into a
			// slot that has just been vacated.
			slices.Reverse(numbers)
		}
		for _, number := range numbers {
			switch {
			case to < from && number >= to && number <= from:
				err = s.renumber(number, number+1)
			case to > from && number <= to && number >= from:
				err = s.renumber(number, number-1)
			}
			if err != nil {
				return err
			}
		}

		return os.Rename(parked, filepath.Join(s.dir, fmt.Sprintf("%05d%s", to, suffix)))
	})
}

// Archive closes a task by moving its file into the done directory.
func (s *Store) Archive(number int) error {
	return s.WithLock(func() error {
		name, err := s.filename(number)
		if err != nil {
			return err
		}
		_, suffix, _ := parseName(filepath.Base(name))
		suffix = strings.TrimSuffix(strings.TrimPrefix(suffix, "-"), ".task")

		done := filepath.Join(s.dir, doneDir)
		if err := os.MkdirAll(done, 0o755); err != nil {
			return fmt.Errorf("could not create done directory: %w", err)
		}

		archived := fmt.Sprintf("%d-%05d-%d-%s.task", time.Now().Unix(), number, os.Getpid(), suffix)
		if err := os.Rename(name, filepath.Join(done, archived)); err != nil {
			return err
		}
		s.invalidate()
		return nil
	})
}

// Delete removes a task file outright, without archiving it.
func (s *Store) Delete(number int) error {
	return s.WithLock(func() error {
		name, err := s.filename(number)
		if err != nil {
			return err
		}
		if err := os.Remove(name); err != nil {
			return err
		}
		s.invalidate()
		return nil
	})
}

// Coalesce closes the gaps in the task numbering, so that the tasks are
// numbered from one with nothing missing in between.
func (s *Store) Coalesce() error {
	return s.WithLock(func() error {
		numbers, err := s.Numbers()
		if err != nil {
			return err
		}

		want := 1
		for _, number := range numbers {
			if number < want {
				continue
			}
			if number > want {
				if err := s.renumber(number, want); err != nil {
					return err
				}
			}
			want++
		}
		return nil
	})
}

// write saves a task, replacing its file atomically so that a task file is
// never left half written.
func (s *Store) write(t *Task) error {
	if t.Number <= 0 || t.Number >= MaxNumber {
		return fmt.Errorf("task number %d out of range", t.Number)
	}
	if t.Title == "" {
		return errors.New("title field not found")
	}
	if t.Created.IsZero() {
		return errors.New("created field not found")
	}

	if t.File == "" {
		t.File = filepath.Join(s.dir, fmt.Sprintf("%05d-none.task", t.Number))
	}

	temp, err := os.CreateTemp(s.dir, ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(t.Marshal()); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), t.File); err != nil {
		return err
	}

	s.invalidate()
	return nil
}

// renumber renames a task file so that it takes a different task number.
func (s *Store) renumber(from, to int) error {
	name, err := s.filename(from)
	if err != nil {
		return err
	}
	_, suffix, _ := parseName(filepath.Base(name))

	if err := os.Rename(name, filepath.Join(s.dir, fmt.Sprintf("%05d%s", to, suffix))); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// filename returns the path of the file holding a task number.
func (s *Store) filename(number int) (string, error) {
	names, err := s.Filenames()
	if err != nil {
		return "", err
	}

	var found []string
	for _, name := range names {
		if n, _, ok := parseName(filepath.Base(name)); ok && n == number {
			found = append(found, name)
		}
	}

	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", fmt.Errorf("%w: %d", ErrNotFound, number)
	default:
		return "", fmt.Errorf("more than one file matches task %d", number)
	}
}

// invalidate drops the cached task list after a change on disk.
func (s *Store) invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cached = nil
}

// readTask loads a single task file.
func readTask(name string) (*Task, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}

	t, err := Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	t.File = name
	t.Number, _, _ = parseName(filepath.Base(name))

	return t, nil
}

// parseName splits a task file name into its number and the remainder of the
// name, which identifies nothing but is preserved across renumbering.
func parseName(base string) (number int, suffix string, ok bool) {
	fields := taskFile.FindStringSubmatch(base)
	if fields == nil {
		return 0, "", false
	}

	number, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, "", false
	}
	return number, fields[2], true
}
