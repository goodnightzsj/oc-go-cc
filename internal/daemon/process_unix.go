//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Send signal 0 to check if process exists without actually signaling it.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// StopProcess sends SIGTERM to a process and waits for it to exit. appName
// guards against a recycled PID: after a crash the PID file can name a process
// that now belongs to something else entirely.
func StopProcess(pid int, appName string) error {
	if appName != "" {
		if name, ok := processIdentity(pid); ok && !processMatches(name, appName) {
			return fmt.Errorf("refusing to stop PID %d: it is %q, not %s", pid, name, appName)
		}
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("cannot find process: %w", err)
	}

	if err := process.Signal(os.Signal(syscall.SIGTERM)); err != nil {
		return fmt.Errorf("cannot send SIGTERM: %w", err)
	}

	// Wait for the process to exit. process.Wait only works for children.
	return waitForExit(pid, 10*time.Second)
}
