package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveExecutablePath_HomebrewStable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Homebrew stable paths are preserved on macOS")
	}
	for _, prefix := range []string{"/opt/homebrew/bin/", "/opt/homebrew/opt/routatic-proxy/bin/", "/usr/local/bin/", "/usr/local/opt/routatic-proxy/bin/"} {
		input := prefix + "./routatic-proxy"
		if got, want := resolveExecutablePath(input), filepath.Clean(input); got != want {
			t.Errorf("resolveExecutablePath(%q) = %q, want stable path %q", input, got, want)
		}
	}
}

func TestResolveExecutablePath_NonHomebrewSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows preserves executable paths without resolving symlinks")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real-binary")
	link := filepath.Join(dir, "binary-link")
	if err := os.WriteFile(target, []byte("test executable"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolveExecutablePath(link); got != want {
		t.Errorf("resolveExecutablePath(%q) = %q, want resolved path %q", link, got, want)
	}
}
