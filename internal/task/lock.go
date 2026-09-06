package task

import (
	"fmt"
	"sync"

	"github.com/gofrs/flock"
)

// fileLock serializes access to the task directory across processes. It is
// counted, so that an operation built out of smaller locking operations holds
// the lock for its whole duration rather than releasing it partway through.
type fileLock struct {
	mu    sync.Mutex
	lock  *flock.Flock
	count int
}

func newFileLock(path string) *fileLock {
	return &fileLock{lock: flock.New(path)}
}

// acquire takes the lock, blocking until it is available.
func (l *fileLock) acquire() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.count == 0 {
		if err := l.lock.Lock(); err != nil {
			return fmt.Errorf("could not lock %s: %w", l.lock.Path(), err)
		}
	}
	l.count++
	return nil
}

// release gives up the lock once every matching acquire has released it.
func (l *fileLock) release() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.count == 0 {
		return fmt.Errorf("lock on %s released when no lock was held", l.lock.Path())
	}

	l.count--
	if l.count == 0 {
		return l.lock.Unlock()
	}
	return nil
}

// held reports how many times the lock has been acquired without release.
func (l *fileLock) held() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.count
}
