package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrLockHeld is returned when an apply lock already exists for the same key.
var ErrLockHeld = errors.New("state lock is already held")

// LockHeldError enriches ErrLockHeld with the conflicting lock file path.
type LockHeldError struct {
	Path string
}

func (e *LockHeldError) Error() string {
	return fmt.Sprintf("%v: %s", ErrLockHeld, e.Path)
}

func (e *LockHeldError) Unwrap() error {
	return ErrLockHeld
}

type Lock struct {
	path       string
	once       sync.Once
	releaseErr error
}

// Path returns the absolute lock file path.
func (l *Lock) Path() string {
	return l.path
}

// Release removes the lock file exactly once and is safe for repeated calls.
func (l *Lock) Release() error {
	l.once.Do(func() {
		err := os.Remove(l.path)
		if err != nil && !os.IsNotExist(err) {
			l.releaseErr = fmt.Errorf("release lock: %w", err)
		}
	})
	return l.releaseErr
}

// AcquireLock creates an exclusive lock for the given cluster/backend pair.
func (s *Store) AcquireLock(clusterName, backend string) (*Lock, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("create state dir for lock: %w", err)
	}
	path := s.lockPath(clusterName, backend)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, &LockHeldError{Path: path}
		}
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	metadata := strings.Join([]string{
		"pid=" + strconv.Itoa(os.Getpid()),
		"time=" + time.Now().UTC().Format(time.RFC3339Nano),
	}, "\n") + "\n"
	if _, err := f.WriteString(metadata); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("write lock metadata: %w", err)
	}

	return &Lock{path: path}, nil
}

// EnsureStateDirAccessible validates read/write/delete access in the state directory.
func (s *Store) EnsureStateDirAccessible() error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	probe := filepath.Join(s.Dir, ".access-check")
	if err := os.WriteFile(probe, []byte("ok\n"), 0o644); err != nil {
		return fmt.Errorf("write state dir probe: %w", err)
	}
	if err := os.Remove(probe); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleanup state dir probe: %w", err)
	}
	return nil
}

func (s *Store) lockPath(clusterName, backend string) string {
	file := fmt.Sprintf("%s--%s.lock", normalizePart(clusterName), normalizePart(backend))
	return filepath.Join(s.Dir, file)
}

// SnapshotPath returns the canonical snapshot path for the given cluster/backend pair.
func (s *Store) SnapshotPath(clusterName, backend string) string {
	return s.path(clusterName, backend)
}
