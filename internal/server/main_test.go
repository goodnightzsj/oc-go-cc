package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/internal/testutil/tokenfixture"
)

func TestMain(m *testing.M) {
	if err := tokenfixture.Prime(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dir, err := os.MkdirTemp("", "oc-go-cc-server-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	// A fixture that omits Storage must never open or prune the user's database.
	storage.DefaultConfig.DatabasePath = filepath.Join(dir, "default.db")
	storage.DefaultConfig.RetentionDays = -1
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}
