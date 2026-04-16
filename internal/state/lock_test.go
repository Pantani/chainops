package state

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquireLockAndRelease(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir)

	lock, err := store.AcquireLock("cluster-a", "docker-compose")
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	if got, want := lock.Path(), filepath.Join(dir, "cluster-a--docker-compose.lock"); got != want {
		t.Fatalf("unexpected lock path: got %q want %q", got, want)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("release lock: %v", err)
	}

	lock2, err := store.AcquireLock("cluster-a", "docker-compose")
	if err != nil {
		t.Fatalf("reacquire lock: %v", err)
	}
	if err := lock2.Release(); err != nil {
		t.Fatalf("release second lock: %v", err)
	}
}

func TestAcquireLockHeld(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir())
	lock, err := store.AcquireLock("cluster-a", "docker-compose")
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	t.Cleanup(func() {
		_ = lock.Release()
	})

	_, err = store.AcquireLock("cluster-a", "docker-compose")
	if err == nil {
		t.Fatal("expected lock contention error")
	}
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld, got %v", err)
	}
	var heldErr *LockHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("expected LockHeldError, got %T", err)
	}
}

func TestEnsureStateDirAccessible(t *testing.T) {
	t.Parallel()

	store := NewStore(filepath.Join(t.TempDir(), "nested", "state"))
	if err := store.EnsureStateDirAccessible(); err != nil {
		t.Fatalf("ensure accessible: %v", err)
	}
}
