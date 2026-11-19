package daemon

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// processMatches reports whether an executable path or name belongs to appName.
// Shared by every platform's identity check, so the naming rule cannot drift.
//
// The suffixes matter: an in-place update renames the running binary to
// "<exe>.old" and unlinks it, after which /proc/<pid>/exe reads
// "routatic-proxy.old (deleted)". Without stripping those, the daemon that
// survived the update no longer recognizes itself and `stop` refuses to stop it.
func processMatches(name, appName string) bool {
	base := strings.ToLower(filepath.Base(name))
	base = strings.TrimSuffix(base, " (deleted)")
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".old")
	app := strings.TrimSuffix(strings.ToLower(appName), ".exe")
	return base == app || strings.HasPrefix(base, app+"-")
}

// waitForExit polls until the process is gone. process.Wait() only works for
// children, and `stop` usually targets a daemon it did not start, where Wait
// returns immediately with "no child processes".
func waitForExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !IsProcessRunning(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("process %d did not exit within %s", pid, timeout)
}
