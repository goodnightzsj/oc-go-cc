//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// WritePID writes the given PID to a file.
// Uses O_NOFOLLOW to atomically reject symlinks at open time,
// preventing symlink-traversal attacks (CWE-59).
func WritePID(pidPath string, pid int) (err error) {
	f, err := os.OpenFile(pidPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return fmt.Errorf("refusing to write PID file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = fmt.Fprintf(f, "%d", pid)
	return err
}
