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

// IsOurProcess reports whether pid is running and belongs to this program.
//
// A bare existence check is not enough for the startup guard: after an unclean
// exit the PID file survives, the system can reuse that PID, and treating the
// stranger as "already running" makes a Restart=always unit loop forever with
// no way in but deleting the file by hand.
func IsOurProcess(pid int, appName string) bool {
	if !IsProcessRunning(pid) {
		return false
	}
	if appName == "" {
		return true
	}
	name, ok := processIdentity(pid)
	if !ok {
		// No identity source on this platform; existence is all we have.
		return true
	}
	return processMatches(name, appName)
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
	// The wait matches the service unit's own stop budget: the proxy drains
	// in-flight streams before it exits, and giving up after ten seconds
	// reported a healthy shutdown as a failed stop.
	return waitForExit(pid, 11*time.Minute)
}
