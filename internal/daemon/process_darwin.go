//go:build darwin

package daemon

import (
	"os/exec"
	"strconv"
	"strings"
)

// IsAppProcess reports whether the PID belongs to this application. Darwin has
// no /proc, so the identity comes from ps; an unreadable identity answers false
// ("unknown"), never "something else".
func IsAppProcess(pid int, appName string) bool {
	name, ok := processIdentity(pid)
	return ok && processMatches(name, appName)
}

func processIdentity(pid int) (string, bool) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", false
	}
	return name, true
}
