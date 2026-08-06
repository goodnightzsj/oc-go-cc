//go:build windows

package daemon

import (
	"fmt"
	"os"
)

// WritePID writes the given PID to a file.
// On Windows, checks for symlinks before writing to prevent
// symlink-traversal attacks (CWE-59). Windows lacks O_NOFOLLOW,
// so we stat the path and refuse to overwrite a symlink.
func WritePID(pidPath string, pid int) error {
	if info, err := os.Lstat(pidPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write PID file: %s is a symlink", pidPath)
	}
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644)
}
