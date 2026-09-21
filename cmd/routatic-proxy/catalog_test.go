package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
	"github.com/spf13/cobra"
)

func TestResolveCatalogDir_Default(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	got := resolveCatalogDir("")
	want := filepath.Join(home, ".config", "routatic-proxy", "catalog")
	if got != want {
		t.Fatalf("resolveCatalogDir(\"\") = %q, want %q", got, want)
	}
}

func TestResolveCatalogDir_FromConfigPath(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.json")

	got := resolveCatalogDir(configPath)
	want := filepath.Join(tmp, "catalog")
	if got != want {
		t.Fatalf("resolveCatalogDir(%q) = %q, want %q", configPath, got, want)
	}
}

func TestCatalogSyncCmd_Help(t *testing.T) {
	cmd := catalogSyncCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Download and cache the models.dev catalog") {
		t.Fatalf("help text missing expected description: %q", out)
	}
}

func TestCatalogCmd_Help(t *testing.T) {
	cmd := catalogCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sync") {
		t.Fatalf("help text missing sync subcommand: %q", out)
	}
}

func TestCatalogSyncCmd_Success(t *testing.T) {
	previousStorage := storage.DefaultConfig
	storage.DefaultConfig.DatabasePath = filepath.Join(t.TempDir(), "catalog.db")
	t.Cleanup(func() { storage.DefaultConfig = previousStorage })
	catalogJSON := `{
  "models": {
    "openrouter/claude-sonnet-4": {"id": "openrouter/claude-sonnet-4", "name": "Claude Sonnet 4"}
  },
  "providers": {
    "openrouter": {"name": "openrouter", "base_url": "https://openrouter.ai/api/v1", "enabled": true}
  }
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, catalogJSON)
	}))
	defer server.Close()

	// Override the package-level source URL for this test.
	oldURL := catalogSourceURL
	catalogSourceURL = server.URL + "/catalog.json"
	t.Cleanup(func() { catalogSourceURL = oldURL })

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	catalogDir := filepath.Join(tmpDir, "catalog")

	root := catalogCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"sync", "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Catalog synced to") {
		t.Fatalf("output missing sync confirmation: %q", out)
	}
	if !strings.Contains(out, "SHA256:") {
		t.Fatalf("output missing SHA256: %q", out)
	}
	if !strings.Contains(out, "Bytes:") {
		t.Fatalf("output missing Bytes: %q", out)
	}
	if !strings.Contains(out, "TTL:") {
		t.Fatalf("output missing TTL: %q", out)
	}

	if _, err := os.Stat(filepath.Join(catalogDir, "catalog.json")); err != nil {
		t.Fatalf("catalog file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(catalogDir, "catalog.lock.json")); err != nil {
		t.Fatalf("lock file not written: %v", err)
	}
}

func TestCatalogSyncCmd_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	oldURL := catalogSourceURL
	catalogSourceURL = server.URL + "/catalog.json"
	t.Cleanup(func() { catalogSourceURL = oldURL })

	root := catalogCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"sync", "--config", filepath.Join(t.TempDir(), "config.json")})

	if err := root.Execute(); err == nil {
		t.Fatal("expected sync to fail on HTTP 500")
	} else if !strings.Contains(err.Error(), "catalog sync failed") {
		t.Fatalf("expected wrapped catalog sync error, got: %v", err)
	}
}

func TestServeCatalog_MissingSyncs(t *testing.T) {
	catalogJSON := `{
  "models": {
    "openrouter/claude-sonnet-4": {"id": "openrouter/claude-sonnet-4", "name": "Claude Sonnet 4"}
  },
  "providers": {
    "openrouter": {"name": "openrouter", "base_url": "https://openrouter.ai/api/v1", "enabled": true}
  }
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, catalogJSON)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	catalogDir := filepath.Join(tmpDir, "catalog")

	cfg := &config.Config{
		Catalog: config.CatalogConfig{
			SourceURL:   server.URL + "/catalog.json",
			MaxAgeHours: 24,
		},
	}

	if err := ensureCatalogSynced(context.Background(), cfg, configPath, time.Now().UTC()); err != nil {
		t.Fatalf("ensureCatalogSynced error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(catalogDir, "catalog.json")); err != nil {
		t.Fatalf("catalog.json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(catalogDir, "catalog.lock.json")); err != nil {
		t.Fatalf("catalog.lock.json not written: %v", err)
	}
}

func TestServeCatalog_ExpiredSyncs(t *testing.T) {
	catalogJSON := `{
  "models": {
    "openrouter/claude-sonnet-4": {"id": "openrouter/claude-sonnet-4", "name": "Claude Sonnet 4"}
  },
  "providers": {
    "openrouter": {"name": "openrouter", "base_url": "https://openrouter.ai/api/v1", "enabled": true}
  }
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/catalog.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, catalogJSON)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	catalogDir := filepath.Join(tmpDir, "catalog")

	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	oldCatalog := []byte("{\"stale\": true}")
	if err := os.WriteFile(filepath.Join(catalogDir, "catalog.json"), oldCatalog, 0644); err != nil {
		t.Fatalf("write old catalog error: %v", err)
	}

	now := time.Now().UTC()
	oldLock := &catalog.Lock{
		SourceURL: server.URL + "/catalog.json",
		SyncedAt:  now.Add(-25 * time.Hour),
		SHA256:    "0000000000000000000000000000000000000000000000000000000000000000",
		Bytes:     int64(len(oldCatalog)),
		TTLHours:  24,
	}
	if err := catalog.WriteLock(catalogDir, oldLock); err != nil {
		t.Fatalf("write old lock error: %v", err)
	}

	cfg := &config.Config{
		Catalog: config.CatalogConfig{
			SourceURL:   server.URL + "/catalog.json",
			MaxAgeHours: 24,
		},
	}

	if err := ensureCatalogSynced(context.Background(), cfg, configPath, now); err != nil {
		t.Fatalf("ensureCatalogSynced error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(catalogDir, "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog error: %v", err)
	}
	if string(data) != catalogJSON {
		t.Fatalf("catalog.json not updated: got %q, want %q", string(data), catalogJSON)
	}

	newLock, err := catalog.ReadLock(catalogDir)
	if err != nil {
		t.Fatalf("read new lock error: %v", err)
	}
	if !newLock.SyncedAt.After(oldLock.SyncedAt) {
		t.Fatalf("lock synced_at not updated: got %v, want after %v", newLock.SyncedAt, oldLock.SyncedAt)
	}
}

func TestServeCatalog_FreshSkipsSync(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	catalogDir := filepath.Join(tmpDir, "catalog")

	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	existingCatalog := []byte("{\"existing\": true}")
	if err := os.WriteFile(filepath.Join(catalogDir, "catalog.json"), existingCatalog, 0644); err != nil {
		t.Fatalf("write existing catalog error: %v", err)
	}

	// Server that would fail if hit; a fresh lock should skip sync entirely.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer server.Close()

	now := time.Now().UTC()
	freshLock := &catalog.Lock{
		SourceURL: server.URL + "/catalog.json",
		SyncedAt:  now.Add(-1 * time.Hour),
		SHA256:    "1111111111111111111111111111111111111111111111111111111111111111",
		Bytes:     int64(len(existingCatalog)),
		TTLHours:  24,
	}
	if err := catalog.WriteLock(catalogDir, freshLock); err != nil {
		t.Fatalf("write fresh lock error: %v", err)
	}

	cfg := &config.Config{
		Catalog: config.CatalogConfig{
			SourceURL:   server.URL + "/catalog.json",
			MaxAgeHours: 24,
		},
	}

	if err := ensureCatalogSynced(context.Background(), cfg, configPath, now); err != nil {
		t.Fatalf("ensureCatalogSynced error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(catalogDir, "catalog.json"))
	if err != nil {
		t.Fatalf("read catalog error: %v", err)
	}
	if string(data) != string(existingCatalog) {
		t.Fatalf("catalog.json changed when lock was fresh: got %q, want %q", string(data), string(existingCatalog))
	}

	lock, err := catalog.ReadLock(catalogDir)
	if err != nil {
		t.Fatalf("read lock error: %v", err)
	}
	if !lock.SyncedAt.Equal(freshLock.SyncedAt) {
		t.Fatalf("lock synced_at changed: got %v, want %v", lock.SyncedAt, freshLock.SyncedAt)
	}
}

func TestCatalogSyncCmd_AddedToRoot(t *testing.T) {
	root := &cobra.Command{Use: "routatic-proxy"}
	root.AddCommand(catalogCmd())

	cmd, args, err := root.Find([]string{"catalog", "sync"})
	if err != nil {
		t.Fatalf("find catalog sync failed: %v", err)
	}
	if cmd.Name() != "sync" {
		t.Fatalf("expected sync command, got %q", cmd.Name())
	}
	if len(args) != 0 {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestServeCatalogRefreshUsesConfiguredSourceAndAge(t *testing.T) {
	for _, reason := range []string{"configured age", "changed source", "missing catalog"} {
		t.Run(reason, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = fmt.Fprint(w, `{"models":{"opencode-go/glm-5":{"id":"glm-5"}},"providers":{"opencode-go":{}}}`)
			}))
			defer upstream.Close()
			path := filepath.Join(t.TempDir(), "config.json")
			dir := resolveCatalogDir(path)
			now := time.Now().UTC()
			lock := &catalog.Lock{SourceURL: upstream.URL, SyncedAt: now.Add(-2 * time.Hour), TTLHours: 24}
			cfg := &config.Config{Catalog: config.CatalogConfig{SourceURL: upstream.URL, MaxAgeHours: 24}}
			if reason == "configured age" {
				cfg.Catalog.MaxAgeHours = 1
			}
			if reason == "changed source" {
				lock.SourceURL += "/previous"
			}
			if err := catalog.WriteLock(dir, lock); err != nil {
				t.Fatal(err)
			}
			if reason != "missing catalog" {
				if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(`{"old":true}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := ensureCatalogSynced(context.Background(), cfg, path, now); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatalf("catalog refreshes = %d, want 1 for %s", calls.Load(), reason)
			}
		})
	}
}

func TestPriceRefreshConfigSyncsWithoutGUIAndFollowsReload(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprint(w, `{"models":{"opencode-go/glm-5":{"id":"glm-5"}},"providers":{"opencode-go":{}}}`)
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := &config.Config{Catalog: config.CatalogConfig{SourceURL: upstream.URL, MaxAgeHours: 24}}
	atomicCfg := config.NewAtomicConfig(cfg, path)
	refresh := priceRefreshConfig(atomicCfg)
	if refresh.CatalogPath != filepath.Join(filepath.Dir(path), "catalog", "catalog.json") {
		t.Fatalf("prices must follow the actual config directory: %s", refresh.CatalogPath)
	}
	for range 2 {
		if err := refresh.RefreshCatalog(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("fresh catalog fetched %d times, want 1", calls.Load())
	}
	updated := *cfg
	updated.Catalog.SourceURL = upstream.URL + "/updated"
	atomicCfg.ApplyLoaded(&updated)
	if err := refresh.RefreshCatalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	lock, err := catalog.ReadLock(resolveCatalogDir(path))
	if err != nil || lock.SourceURL != updated.Catalog.SourceURL || calls.Load() != 2 {
		t.Fatalf("reloaded source not synced: lock=%+v calls=%d err=%v", lock, calls.Load(), err)
	}
	disabled := updated
	disabled.Catalog.Enabled = new(bool)
	atomicCfg.ApplyLoaded(&disabled)
	if err := refresh.RefreshCatalog(context.Background()); err != nil || calls.Load() != 2 {
		t.Fatalf("disabled sync made a network request: calls=%d err=%v", calls.Load(), err)
	}
}
