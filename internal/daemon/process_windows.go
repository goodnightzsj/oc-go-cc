//go:build windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const windowsSynchronize = 0x00100000

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	handle, err := syscall.OpenProcess(windowsSynchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	event, err := syscall.WaitForSingleObject(handle, 0)
	return err == nil && event == syscall.WAIT_TIMEOUT
}

func IsAppProcess(pid int, appName string) bool {
	name, ok := processIdentity(pid)
	return ok && processMatches(name, appName)
}

func processIdentity(pid int) (string, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:size]), true
}

// StopProcess terminates a process on Windows.
// Unlike the Unix implementation which sends SIGTERM for graceful shutdown,
// this uses process.Kill() (TerminateProcess) which immediately terminates the
// process without cleanup. In-flight requests are dropped and deferred functions
// do not run. A future improvement could use a named pipe or event for graceful
// shutdown.
// IsOurProcess reports whether pid is running and belongs to this program.
// See the Unix implementation for why existence alone is not enough.
func IsOurProcess(pid int, appName string) bool {
	if !IsProcessRunning(pid) {
		return false
	}
	if appName == "" {
		return true
	}
	name, ok := processIdentity(pid)
	if !ok {
		return true
	}
	return processMatches(name, appName)
}

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

	if err := process.Kill(); err != nil {
		return fmt.Errorf("cannot terminate process: %w", err)
	}

	return waitForExit(pid, 10*time.Second)
}
