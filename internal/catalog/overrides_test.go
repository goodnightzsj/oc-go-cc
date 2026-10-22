package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/storage"
)

// The mirror does not publish this model under cline-pass, so the import must
// add it - and must add it in a way the next sync cannot drop.
func TestImportAddsPlatformModelTheMirrorOmits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {
	    "cline-pass": {"name": "cline-pass", "enabled": true,
	      "models": {"deepseek-v4.1-flash": {"name": "DeepSeek V4.1 Flash", "limit": {"context": 1000000, "output": 384000}, "tool_call": true, "modalities": {"input": ["text","image"], "output": ["text"]}}}},
	    "deepseek": {"name": "deepseek", "enabled": true}
	  },
	  "models": {
	    "deepseek/deepseek-v4.1-flash-1017": {"name": "DeepSeek V4.1 Flash 1017", "limit": {"context": 1000000, "output": 384000}, "tool_call": true, "modalities": {"input": ["text","image"], "output": ["text"]}}
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(dir, "d.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := ImportFromJSON(context.Background(), db, path); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadFromSQLite(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}

	const key = "cline-pass/deepseek-v4.1-flash-1017"
	model, ok := idx.Models[key]
	if !ok {
		t.Fatalf("%s missing after import: the platform serves it but the mirror does not file it under cline-pass", key)
	}
	if !model.SupportsVision() {
		t.Error("the platform answers images sent to this model")
	}
	if model.ContextWindow() != 1000000 {
		t.Errorf("context = %d, want 1000000", model.ContextWindow())
	}
	if resolved, err := idx.Resolve(Selector{Provider: "cline-pass", Model: "deepseek-v4.1-flash-1017"}); err != nil {
		t.Errorf("cannot resolve under the platform's own name: %v", err)
	} else if !resolved.Vision {
		t.Error("resolved without vision")
	}

	// A second import must land on the same answer, not accumulate or drop it.
	if _, _, err := ImportFromJSON(context.Background(), db, path); err != nil {
		t.Fatal(err)
	}
	idx2, err := LoadFromSQLite(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx2.Models[key]; !ok {
		t.Fatal("a second sync dropped the entry")
	}
}

// A mirror entry always wins, so this table cannot silently shadow a real one.
func TestOverrideNeverReplacesAPublishedModel(t *testing.T) {
	cat := &Catalog{
		Providers: map[string]Provider{"cline-pass": {Name: "cline-pass"}},
		Models: map[string]Model{
			"cline-pass/deepseek-v4.1-flash-1017": {
				Name:  "Mirror Says Something Else",
				Limit: &Limit{Context: 111, Output: 222},
			},
		},
	}
	applyProviderOverrides(cat)
	if got := cat.Models["cline-pass/deepseek-v4.1-flash-1017"].ContextWindow(); got != 111 {
		t.Errorf("context = %d, want the mirror's 111 - a published entry must win", got)
	}
}

// overrideEntryCount reports how many entries the table adds for a platform.
func overrideEntryCount(provider string) int {
	return len(providerModelOverrides[provider])
}

// The dashboard reads the JSON catalog directly, while routing reads what the
// importer wrote to SQLite. Applying the override table in only one of those
// places left the picker offering a roster that did not match what the proxy
// accepts - the platform's own 1017 model was missing from it.
func TestLoadAppliesTheOverrideTableToo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
	  "providers": {
	    "cline-pass": {"name": "cline-pass", "enabled": true},
	    "other": {"name": "other", "enabled": true}
	  },
	  "models": {"other/m": {"name": "M", "limit": {"context": 1, "output": 1}}}
	}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	const key = "cline-pass/deepseek-v4.1-flash-1017"
	if _, ok := idx.Models[key]; !ok {
		t.Fatalf("%s is missing: the dashboard would offer a roster that excludes a model the proxy serves", key)
	}
	// It also has to appear in the provider's own index, not only in the map.
	found := false
	for _, m := range idx.ListProviderModels("cline-pass") {
		if m.CanonicalName == key {
			found = true
		}
	}
	if !found {
		t.Error("the entry is resolvable by key but absent from the provider's listing")
	}
}
