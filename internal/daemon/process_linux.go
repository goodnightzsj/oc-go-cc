//go:build linux

package daemon

import (
	"os"
	"path/filepath"
	"strconv"
)

// IsAppProcess reports whether the PID belongs to this application. It answers
// false when the identity cannot be read, so callers must treat false as
// "unknown", not as a positive identification of something else.
func IsAppProcess(pid int, appName string) bool {
	name, ok := processIdentity(pid)
	return ok && processMatches(name, appName)
}

func processIdentity(pid int) (string, bool) {
	exe, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err != nil {
		return "", false
	}
	return exe, true
}
