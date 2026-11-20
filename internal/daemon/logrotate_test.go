package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterBoundsTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy.log")
	// A file that is already past the limit must be rotated on the first write,
	// not left to grow: that is the state every existing install starts in.
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0644); err != nil {
		t.Fatal(err)
	}
	w, err := NewRotatingWriter(path, 40)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path + ".1"); err != nil || len(data) != 100 {
		t.Fatalf("previous generation not kept: %d bytes, %v", len(data), err)
	}
	for i := 0; i < 20; i++ {
		if _, err := w.Write([]byte("0123456789\n")); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 40+11 {
		t.Fatalf("log grew past the limit: %d bytes", info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "0123456789") {
		t.Fatalf("rotation lost the current line: %q %v", data, err)
	}
}
