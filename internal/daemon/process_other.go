//go:build !windows && !linux && !darwin

package daemon

// IsAppProcess cannot read another process's identity on this platform, so it
// answers false: callers treat that as unknown and keep their previous
// behaviour.
func IsAppProcess(pid int, appName string) bool { return false }

func processIdentity(pid int) (string, bool) { return "", false }
