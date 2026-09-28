package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// ErrLocked reports that another process holds the task directory. It is
// returned rather than waited out forever, so that a peer sitting in an editor
// or a pager cannot hang this process for as long as its user takes to finish.
var ErrLocked = errors.New("another task process is using the task directory")

// lockWait is how long a caller waits for another process to give up the task
// directory before giving up with ErrLocked.
const lockWait = 2 * time.Second

// lockRetry is how often the lock is retried while waiting.
const lockRetry = 20 * time.Millisecond

// fileLock serializes access to the task directory across processes. It is
// counted, so that an operation built out of smaller locking operations holds
// the lock for its whole duration rather than releasing it partway through.
type fileLock struct {
	mu    sync.Mutex
	ready *sync.Cond
	lock  *flock.Flock
	count int
	// busy is set while the file lock itself is being taken or given up,
	// which is done with mu released so that waiting on another process
	// does not also block the goroutines of this one.
	busy bool
}

func newFileLock(path string) *fileLock {
	l := &fileLock{lock: flock.New(path)}
	l.ready = sync.NewCond(&l.mu)

	return l
}

// acquire takes the lock, waiting at most wait for another process to release
// it and returning ErrLocked if that runs out. A wait of zero does not wait at
// all. A lock this process already holds is taken again immediately, however
// long the wait.
func (l *fileLock) acquire(wait time.Duration) error {
	l.mu.Lock()
	for l.busy {
		l.ready.Wait()
	}
	if l.count > 0 {
		l.count++
		l.mu.Unlock()
		return nil
	}
	l.busy = true
	l.mu.Unlock()

	err := l.take(wait)

	l.mu.Lock()
	l.busy = false
	if err == nil {
		l.count = 1
	}
	l.ready.Broadcast()
	l.mu.Unlock()

	return err
}

// release gives up the lock once every matching acquire has released it.
func (l *fileLock) release() error {
	l.mu.Lock()
	for l.busy {
		l.ready.Wait()
	}
	if l.count == 0 {
		l.mu.Unlock()
		return fmt.Errorf("lock on %s released when no lock was held", l.lock.Path())
	}
	if l.count > 1 {
		l.count--
		l.mu.Unlock()
		return nil
	}
	l.count = 0
	l.busy = true
	l.mu.Unlock()

	err := l.lock.Unlock()

	l.mu.Lock()
	l.busy = false
	l.ready.Broadcast()
	l.mu.Unlock()

	return err
}

// held reports how many times the lock has been acquired without release.
func (l *fileLock) held() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.count
}

// take blocks on the file lock, with mu released.
func (l *fileLock) take(wait time.Duration) error {
	if wait <= 0 {
		ok, err := l.lock.TryLock()
		if err != nil {
			return fmt.Errorf("could not lock %s: %w", l.lock.Path(), err)
		}
		if !ok {
			return fmt.Errorf("%s: %w", l.lock.Path(), ErrLocked)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	ok, err := l.lock.TryLockContext(ctx, lockRetry)
	switch {
	case ok:
		return nil
	case err == nil, errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s: %w", l.lock.Path(), ErrLocked)
	default:
		return fmt.Errorf("could not lock %s: %w", l.lock.Path(), err)
	}
}
