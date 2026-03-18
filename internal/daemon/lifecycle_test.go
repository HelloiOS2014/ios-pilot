package daemon

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPIDFileWriteRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := WritePIDFile(path); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	pid, err := ReadPID(path)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("PID mismatch: got %d, want %d", pid, os.Getpid())
	}
}

func TestIsRunning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := WritePIDFile(path); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	if !IsRunning(path) {
		t.Error("IsRunning: expected true for current process, got false")
	}
}

func TestIsRunningFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	// Find a PID that doesn't exist. We probe from a high number downward;
	// writing a non-existent PID directly is simpler.
	// PID 99999999 is extremely unlikely to be a running process.
	const fakePID = 99999999
	if err := os.WriteFile(path, []byte("99999999\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Make sure this PID is actually not running on this machine.
	if err := syscall.Kill(fakePID, 0); err == nil {
		t.Skip("PID 99999999 exists on this machine, skipping")
	}

	if IsRunning(path) {
		t.Error("IsRunning: expected false for non-existent PID, got true")
	}
}

func TestIsRunningNoPIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.pid")

	if IsRunning(path) {
		t.Error("IsRunning: expected false for missing PID file, got true")
	}
}

func TestFileLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	// Acquire lock.
	f1, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireLock (first): %v", err)
	}
	defer ReleaseLock(f1)

	// Attempt to acquire the same lock again from the same process.
	// flock with LOCK_NB should fail.
	f2, err := AcquireLock(lockPath)
	if err == nil {
		ReleaseLock(f2)
		t.Fatal("AcquireLock (second): expected error, got nil")
	}

	// Release first lock and try again — should succeed.
	ReleaseLock(f1)

	f3, err := AcquireLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireLock (after release): %v", err)
	}
	ReleaseLock(f3)
}

func TestIdleTimer(t *testing.T) {
	fired := make(chan struct{}, 1)
	it := NewIdleTimer(50*time.Millisecond, func() {
		fired <- struct{}{}
	})
	defer it.Stop()

	select {
	case <-fired:
		// Good — timer fired as expected.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("IdleTimer: callback did not fire within deadline")
	}
}

func TestIdleTimerReset(t *testing.T) {
	fired := make(chan struct{}, 1)
	it := NewIdleTimer(80*time.Millisecond, func() {
		fired <- struct{}{}
	})
	defer it.Stop()

	// Reset before the timer fires a few times.
	for i := 0; i < 3; i++ {
		time.Sleep(50 * time.Millisecond)
		it.Reset()
	}

	// Timer should have been deferred — it should NOT have fired yet.
	select {
	case <-fired:
		t.Fatal("IdleTimer fired too early after repeated Reset()")
	default:
	}

	// Now let it actually expire.
	select {
	case <-fired:
		// Good.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("IdleTimer: callback did not fire after stop resetting")
	}
}

func TestIdleTimerStop(t *testing.T) {
	fired := make(chan struct{}, 1)
	it := NewIdleTimer(50*time.Millisecond, func() {
		fired <- struct{}{}
	})
	it.Stop()

	select {
	case <-fired:
		t.Fatal("IdleTimer: callback fired after Stop()")
	case <-time.After(200 * time.Millisecond):
		// Good — timer was stopped.
	}
}

func TestRemovePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := WritePIDFile(path); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	RemovePIDFile(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("PID file should have been removed")
	}

	// Calling again should not panic.
	RemovePIDFile(path)
}
