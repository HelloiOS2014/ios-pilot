// Package daemon implements the ios-pilot daemon infrastructure:
// Unix socket JSON-RPC server, PID file management, file locking, and idle timer.
package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// WritePIDFile writes the current process PID to path.
func WritePIDFile(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// RemovePIDFile removes the PID file at path, ignoring errors.
func RemovePIDFile(path string) {
	os.Remove(path)
}

// ReadPID reads and returns the PID stored in path.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file %q: %w", path, err)
	}
	return pid, nil
}

// IsRunning returns true if a process with the PID recorded in pidPath is alive.
// Returns false if the file cannot be read or the process does not exist.
func IsRunning(pidPath string) bool {
	pid, err := ReadPID(pidPath)
	if err != nil {
		return false
	}
	// syscall.Kill(pid, 0) returns nil if process exists and we can signal it.
	err = syscall.Kill(pid, 0)
	return err == nil
}

// AcquireLock opens lockPath (creating it if necessary) and acquires an
// exclusive advisory lock via flock(2). Returns the open file on success.
// Returns an error if the lock is already held by another process.
func AcquireLock(lockPath string) (*os.File, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	return f, nil
}

// ReleaseLock releases the flock and closes f.
func ReleaseLock(f *os.File) {
	unix.Flock(int(f.Fd()), unix.LOCK_UN) //nolint:errcheck
	f.Close()
}

// IdleTimer fires onExpire after duration of inactivity.
// Call Reset() on each incoming request to defer expiry.
type IdleTimer struct {
	duration time.Duration
	onExpire func()
	timer    *time.Timer
}

// NewIdleTimer creates and starts an idle timer that calls onExpire after
// duration of inactivity.
func NewIdleTimer(duration time.Duration, onExpire func()) *IdleTimer {
	it := &IdleTimer{
		duration: duration,
		onExpire: onExpire,
	}
	it.timer = time.AfterFunc(duration, onExpire)
	return it
}

// Reset restarts the idle timer countdown.
func (it *IdleTimer) Reset() {
	it.timer.Reset(it.duration)
}

// Stop cancels the idle timer without firing onExpire.
func (it *IdleTimer) Stop() {
	it.timer.Stop()
}
