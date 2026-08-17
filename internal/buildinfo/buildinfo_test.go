package buildinfo

import (
	"testing"
)

func TestVersionNotEmpty(t *testing.T) {
	if Version == "" {
		t.Error("Version must not be empty")
	}
}

func TestDateNotEmpty(t *testing.T) {
	if Date == "" {
		t.Error("Date must not be empty")
	}
}

func TestBinaryPathReturnsSomething(t *testing.T) {
	p := BinaryPath()
	if p == "" {
		t.Error("BinaryPath() returned empty string")
	}
}

func TestInitDoesNotPanic(t *testing.T) {
	// init() has already run; just ensure the package level vars are sane.
	if Version == "" || Commit == "" || Date == "" {
		t.Fatal("expected non-empty build info after package init")
	}
}
