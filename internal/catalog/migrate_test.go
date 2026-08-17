package catalog

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

const testCatalogFixture = `{
  "providers": {
    "opencode-go": {"name": "opencode-go", "base_url": "https://opencode.ai/zen/go/v1/chat/completions", "api_key": "must-not-persist", "enabled": true},
    "opencode-zen": {"name": "opencode-zen", "base_url": "https://opencode.ai/zen/v1/chat/completions", "enabled": true}
  },
  "models": {
    "opencode-go/model-go": {"id": "opencode-go/model-go", "name": "Model Go"},
    "opencode-zen/model-zen": {"id": "opencode-zen/model-zen", "name": "Model Zen"}
  }
}`

func TestMigrateFromJSON(t *testing.T) {
	tmp := t.TempDir()

	catalogDir := filepath.Join(tmp, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("mkdir catalog: %v", err)
	}
	jsonPath := filepath.Join(catalogDir, "catalog.json")
	if err := os.WriteFile(jsonPath, []byte(testCatalogFixture), 0644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	dbPath := filepath.Join(tmp, "data.db")
	storageCfg := storage.DefaultConfig
	storageCfg.DatabasePath = dbPath

	db, err := storage.Open(storageCfg)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	start := time.Now()
	providers, models, err := ImportFromJSON(ctx, db, jsonPath)
	elapsed := time.Since(start)

	t.Logf("ImportFromJSON took: %v", elapsed)

	if err != nil {
		t.Fatalf("ImportFromJSON: %v", err)
	}
	if providers == 0 || models == 0 {
		t.Fatalf("expected a non-empty import, got %d providers and %d models", providers, models)
	}

	idx, err := LoadFromSQLite(ctx, db)
	if err != nil {
		t.Fatalf("LoadFromSQLite: %v", err)
	}

	if len(idx.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(idx.Providers))
	}
	if len(idx.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(idx.Models))
	}

	var persistedKey *string
	if err := db.DB().QueryRowContext(ctx, `SELECT api_key FROM providers WHERE name = 'opencode-go'`).Scan(&persistedKey); err != nil {
		t.Fatalf("read persisted catalog API key: %v", err)
	}
	if persistedKey != nil {
		t.Errorf("persisted catalog API key = %q, want NULL", *persistedKey)
	}
}

// ImportFromJSON must refresh the SQLite catalog every time, not only on the
// first run. It used to bail out once catalog_last_sync was set, so every
// `routatic-proxy catalog sync` after the first downloaded a fresh catalog and
// then left SQLite — the copy production routing reads — on the original
// snapshot, while reporting success.
func TestImportFromJSON_RefreshesAnExistingCatalog(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(dir, "catalog.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	jsonPath := filepath.Join(dir, "catalog.json")
	writeCatalog := func(provider, modelKey string) {
		t.Helper()
		body := `{"providers":{"` + provider + `":{"name":"` + provider + `"}},` +
			`"models":{"` + provider + `/` + modelKey + `":{"id":"` + provider + `/` + modelKey +
			`","name":"` + modelKey + `","limit":{"context":1000},"cost":{"input":1,"output":1}}}}`
		if err := os.WriteFile(jsonPath, []byte(body), 0o644); err != nil {
			t.Fatalf("write catalog: %v", err)
		}
	}

	ctx := context.Background()

	writeCatalog("opencode-go", "model-old")
	if _, models, err := ImportFromJSON(ctx, db, jsonPath); err != nil || models != 1 {
		t.Fatalf("first import: models=%d err=%v", models, err)
	}

	// A later `catalog sync` lands a different catalog on disk.
	writeCatalog("opencode-zen", "model-new")
	if _, models, err := ImportFromJSON(ctx, db, jsonPath); err != nil || models != 1 {
		t.Fatalf("second import: models=%d err=%v", models, err)
	}

	idx, err := LoadFromSQLite(ctx, db)
	if err != nil {
		t.Fatalf("load from SQLite: %v", err)
	}
	if _, ok := idx.Models["opencode-zen/model-new"]; !ok {
		t.Errorf("SQLite is stale: model-new missing after re-import, holds %v", slices.Sorted(maps.Keys(idx.Models)))
	}
	if _, ok := idx.Models["opencode-go/model-old"]; ok {
		t.Errorf("stale model survived replacement: %v", slices.Sorted(maps.Keys(idx.Models)))
	}
	if _, ok := idx.Providers["opencode-go"]; ok {
		t.Errorf("stale provider survived replacement: %v", slices.Sorted(maps.Keys(idx.Providers)))
	}
}
