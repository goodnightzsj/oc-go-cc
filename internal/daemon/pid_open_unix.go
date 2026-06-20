//go:build !windows

package daemon

import "syscall"

const nofollowFlag = syscall.O_NOFOLLOW
