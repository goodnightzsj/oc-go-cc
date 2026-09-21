package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSync(t *testing.T) {
	validCatalog := `{"models":{"openai/gpt-4":{"id":"openai/gpt-4","name":"gpt-4"}},"providers":{"openai":{}}}`
	validHash := sha256.Sum256([]byte(validCatalog))

	cases := []struct {
		name        string
		body        string
		wantErr     bool
		wantLock    bool
		wantCatalog bool
	}{
		{
			name:        "successful sync writes catalog and lock",
			body:        validCatalog,
			wantErr:     false,
			wantLock:    true,
			wantCatalog: true,
		},
		{
			name:        "missing providers object returns error and leaves no catalog",
			body:        `{"models":{"openai/gpt-4":{"id":"openai/gpt-4"}}}`,
			wantErr:     true,
			wantLock:    false,
			wantCatalog: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			destDir := t.TempDir()
			lock, err := Sync(context.Background(), server.URL, destDir)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			if tc.wantLock && lock == nil {
				t.Fatalf("expected non-nil lock")
			}
			if !tc.wantLock && lock != nil {
				t.Fatalf("expected nil lock, got %+v", lock)
			}

			catalogPath := filepath.Join(destDir, catalogFileName)
			if tc.wantCatalog {
				data, err := os.ReadFile(catalogPath)
				if err != nil {
					t.Fatalf("expected catalog file: %v", err)
				}
				if string(data) != tc.body {
					t.Fatalf("catalog content mismatch: got %q, want %q", string(data), tc.body)
				}
			} else {
				if _, err := os.Stat(catalogPath); !os.IsNotExist(err) {
					t.Fatalf("expected no catalog file, got %v", err)
				}
			}

			lockPath := filepath.Join(destDir, lockFileName)
			if tc.wantLock {
				read, err := ReadLock(destDir)
				if err != nil {
					t.Fatalf("expected readable lock: %v", err)
				}
				if read.SourceURL != server.URL {
					t.Fatalf("lock source URL mismatch: got %q, want %q", read.SourceURL, server.URL)
				}
				if read.SHA256 != hex.EncodeToString(validHash[:]) {
					t.Fatalf("lock SHA256 mismatch: got %q, want %q", read.SHA256, hex.EncodeToString(validHash[:]))
				}
				if read.Bytes != int64(len(tc.body)) {
					t.Fatalf("lock bytes mismatch: got %d, want %d", read.Bytes, len(tc.body))
				}
				if read.TTLHours != defaultTTLHours {
					t.Fatalf("lock TTL mismatch: got %d, want %d", read.TTLHours, defaultTTLHours)
				}
			} else {
				if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
					t.Fatalf("expected no lock file, got %v", err)
				}
			}

			tmpPath := filepath.Join(destDir, tmpFileName)
			if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
				t.Fatalf("expected no temp file left behind, got %v", err)
			}
		})
	}
}

func TestSyncOversized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		prefix := []byte(`{"models":{`)
		_, _ = w.Write(prefix)
		padding := strings.Repeat("0", maxCatalogBytes+1)
		_, _ = w.Write([]byte(padding))
		_, _ = w.Write([]byte(`},"providers":{}}`))
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, err := Sync(context.Background(), server.URL, destDir)
	if err == nil {
		t.Fatalf("expected error for oversized response, got nil")
	}

	for _, name := range []string{catalogFileName, tmpFileName, lockFileName} {
		path := filepath.Join(destDir, name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected no %s after oversized sync failure, got %v", name, err)
		}
	}
}

func TestSyncNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, err := Sync(context.Background(), server.URL, destDir)
	if err == nil {
		t.Fatalf("expected error for non-OK status, got nil")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", http.StatusInternalServerError)) {
		t.Fatalf("expected status in error, got %v", err)
	}
}

func TestSyncMissingModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":{"openai":{}}}`))
	}))
	defer server.Close()

	destDir := t.TempDir()
	_, err := Sync(context.Background(), server.URL, destDir)
	if err == nil {
		t.Fatalf("expected error for missing models object, got nil")
	}
	if _, err := os.Stat(filepath.Join(destDir, catalogFileName)); !os.IsNotExist(err) {
		t.Fatalf("expected no catalog file, got %v", err)
	}
}

func TestSyncCreatesDestDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"y/x":{"id":"y/x","name":"x"}},"providers":{"y":{}}}`))
	}))
	defer server.Close()

	base := t.TempDir()
	destDir := filepath.Join(base, "nested", "catalog")
	_, err := Sync(context.Background(), server.URL, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, catalogFileName)); err != nil {
		t.Fatalf("expected catalog file: %v", err)
	}
}

func TestSyncCancellationPreservesExistingCatalog(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, catalogFileName)
	previous := []byte(`{"previous":true}`)
	if err := os.WriteFile(path, previous, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Sync(ctx, upstream.URL, dir)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("catalog fetch did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("sync cancellation = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("catalog sync ignored cancellation")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(previous) {
		t.Fatalf("cancelled sync replaced the previous catalog: %q, %v", data, err)
	}
}

func TestConcurrentSyncKeepsCatalogAndLockPaired(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"models":{"opencode-go/m":{"id":%q}},"providers":{"opencode-go":{}}}`, r.URL.Path)
	}))
	defer upstream.Close()
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Sync(context.Background(), fmt.Sprintf("%s/%d", upstream.URL, i), dir); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(dir, catalogFileName))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := ReadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if lock.SHA256 != hex.EncodeToString(sum[:]) || lock.Bytes != int64(len(data)) {
		t.Fatalf("catalog and lock came from different syncs: %+v", lock)
	}
}
