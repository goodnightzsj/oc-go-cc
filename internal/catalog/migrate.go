package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/routatic/proxy/internal/storage"
)

// DirForConfig returns the catalog directory that belongs to a config file.
// The proxy, the CLI and the dashboard resolve it here so a catalog downloaded
// by one of them is the one the others read.
func DirForConfig(configPath string) string {
	if configPath != "" {
		return filepath.Join(filepath.Dir(configPath), "catalog")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "routatic-proxy", "catalog")
	}
	return filepath.Join(home, ".config", "routatic-proxy", "catalog")
}

// EnsureImported brings the SQLite catalog up to date with a catalog.json that
// was downloaded after the last import. Startup and the dashboard both
// download the file; without this step routing and cost selection kept reading
// the previous snapshot until someone happened to run `catalog sync`.
func EnsureImported(ctx context.Context, db *storage.Database, jsonPath string) error {
	info, err := os.Stat(jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no downloaded catalog; nothing to import
		}
		return err
	}
	repo := storage.NewCatalogRepo(db)
	last, err := repo.LastSync(ctx)
	if err != nil {
		return err
	}

	// The sync lock records the hash of the file it wrote. That is the file's
	// identity: mtime alone can be older for newer content (an rsync -t
	// restore, or a second config sharing this database), which would skip a
	// real update forever.
	sha := ""
	if lock, err := ReadLock(filepath.Dir(jsonPath)); err == nil {
		sha = lock.SHA256
	}
	if sha != "" {
		if stored, err := repo.ImportedSHA(ctx); err == nil && stored == sha {
			return nil
		}
	} else if !last.IsZero() && !info.ModTime().Truncate(time.Second).After(last) {
		// Without a hash, fall back to the import timestamp at the stored
		// precision, so sub-second mtime digits do not force a re-import.
		return nil
	}

	if _, _, err := ImportFromJSON(ctx, db, jsonPath); err != nil {
		return err
	}
	if sha != "" {
		// A failed hash write only means the next start imports once more; it
		// must not turn a successful import into an error.
		_ = repo.SetImportedSHA(ctx, sha)
	}
	return nil
}

// ImportFromJSON reads catalog.json and replaces the SQLite catalog with its
// contents, returning the number of providers and models imported.
//
// This is deliberately unconditional. It previously bailed out whenever the
// catalog had ever been imported, which meant `routatic-proxy catalog sync`
// downloaded a fresh catalog and then silently kept serving the first snapshot
// it ever saw — including to cost-based routing, which picks models by price.
func ImportFromJSON(ctx context.Context, db *storage.Database, jsonPath string) (providerCount, modelCount int, err error) {
	repo := storage.NewCatalogRepo(db)

	idx, err := Load(jsonPath)
	if err != nil {
		return 0, 0, fmt.Errorf("load catalog from JSON: %w", err)
	}

	// Applied here rather than in Load so a test fixture is exactly the file it
	// wrote. This is the entry point every sync goes through, and it replaces
	// the whole table, so an entry added here survives the mirror's next
	// download - which is the only requirement.
	applyProviderOverrides(&idx.Catalog)

	providers := make([]storage.ProviderRecord, 0, len(idx.Providers))
	for name, p := range idx.Providers {
		providers = append(providers, providerToStorageRecord(name, p))
	}

	models := make([]storage.ModelRecord, 0, len(idx.Models))
	for key, m := range idx.Models {
		models = append(models, modelToStorageRecord(key, m))
	}

	if err := repo.ReplaceBatch(ctx, providers, models); err != nil {
		return 0, 0, fmt.Errorf("import catalog to SQLite: %w", err)
	}

	return len(providers), len(models), nil
}

// ExportJSON exports SQLite catalog to JSON for backup/debugging.
func ExportJSON(ctx context.Context, db *storage.Database, jsonPath string) error {
	repo := storage.NewCatalogRepo(db)

	idx, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("load catalog from SQLite: %w", err)
	}

	catalog := &Catalog{
		Providers: make(map[string]Provider, len(idx.Providers)),
		Models:    make(map[string]Model, len(idx.Models)),
	}

	for name, p := range idx.Providers {
		catalog.Providers[name] = Provider{
			Name:                   p.Name,
			BaseURL:                p.BaseURL,
			Enabled:                p.Enabled,
			AnthropicToolsDisabled: p.AnthropicToolsDisabled,
		}
	}

	for key, m := range idx.Models {
		model := storageModelToCatalogModel(m)
		model.ID = WireModelID(key)
		catalog.Models[key] = model
	}

	return WriteFile(jsonPath, catalog)
}

// LoadFromSQLite loads the catalog from SQLite and returns an IndexedCatalog.
func LoadFromSQLite(ctx context.Context, db *storage.Database) (*IndexedCatalog, error) {
	repo := storage.NewCatalogRepo(db)

	storageIdx, err := repo.Load(ctx)
	if err != nil {
		return nil, err
	}

	cat := &Catalog{
		Providers: make(map[string]Provider, len(storageIdx.Providers)),
		Models:    make(map[string]Model, len(storageIdx.Models)),
	}

	for name, p := range storageIdx.Providers {
		cat.Providers[name] = Provider{
			Name:                   p.Name,
			BaseURL:                p.BaseURL,
			Enabled:                p.Enabled,
			AnthropicToolsDisabled: p.AnthropicToolsDisabled,
		}
	}

	for key, m := range storageIdx.Models {
		model := storageModelToCatalogModel(m)
		model.ID = WireModelID(key)
		cat.Models[key] = model
	}

	idx := &IndexedCatalog{
		Catalog:        *cat,
		ProviderModels: make(map[string][]Model, len(storageIdx.ProviderModels)),
	}

	for prov, models := range storageIdx.ProviderModels {
		converted := make([]Model, len(models))
		for i, m := range models {
			converted[i] = storageModelToCatalogModel(m)
		}
		idx.ProviderModels[prov] = converted
	}

	return idx, nil
}

func providerToStorageRecord(name string, p Provider) storage.ProviderRecord {
	return storage.ProviderRecord{
		Name:                   name,
		BaseURL:                p.BaseURL,
		Enabled:                p.Enabled,
		AnthropicToolsDisabled: p.AnthropicToolsDisabled,
	}
}

func modelToStorageRecord(key string, m Model) storage.ModelRecord {
	record := storage.ModelRecord{
		ID:              key,
		Name:            m.Name,
		Reasoning:       m.Reasoning,
		ToolCall:        m.ToolCall,
		Vision:          m.SupportsVision(),
		ContextWindow:   m.ContextWindow(),
		MaxOutputTokens: m.MaxOutputTokens(),
	}
	if r := m.Rates(); r != nil {
		record.Rates = &storage.Rates{Input: r.Input, Output: r.Output}
	}
	for _, t := range m.PromptTiers() {
		record.Tiers = append(record.Tiers, storage.PromptTier{
			MinPromptTokens: t.MinPromptTokens, Input: t.Input, Output: t.Output, CacheRead: t.CacheRead,
		})
	}
	return record
}

func storageModelToCatalogModel(m storage.Model) Model {
	model := Model{
		ID:        m.ID,
		Name:      m.Name,
		Reasoning: m.Reasoning,
		ToolCall:  m.ToolCall,
	}
	if m.Vision {
		model.Modalities.Input = []string{"text", "image"}
	} else {
		model.Modalities.Input = []string{"text"}
	}
	model.Modalities.Output = []string{"text"}
	if m.Limit != nil {
		model.Limit = &Limit{Context: m.Limit.Context, Output: m.Limit.Output}
	}
	if m.Rates != nil {
		cost := &Cost{Input: m.Rates.Input, Output: m.Rates.Output}
		for _, t := range m.Tiers {
			cost.Overrides = append(cost.Overrides, CostOverride{
				MinPromptTokens: t.MinPromptTokens, Prompt: t.Input, Completion: t.Output,
				InputCacheRead: t.CacheRead,
			})
		}
		model.Cost = cost
	}
	return model
}
