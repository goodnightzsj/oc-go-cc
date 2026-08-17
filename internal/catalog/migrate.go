package catalog

import (
	"context"
	"fmt"

	"github.com/routatic/proxy/internal/storage"
)

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
		model.ID = ModelNameFromKey(key)
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
		model.ID = ModelNameFromKey(key)
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
	return storage.ModelRecord{
		ID:            key,
		Name:          m.Name,
		Reasoning:     m.Reasoning,
		ToolCall:      m.ToolCall,
		Vision:        m.SupportsVision(),
		ContextWindow: m.ContextWindow(),
		CostInput:     m.CostInputPerM(),
		CostOutput:    m.CostOutputPerM(),
	}
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
		model.Limit = &Limit{Context: m.Limit.Context}
	}
	if m.Rates != nil {
		model.Rates = &Rates{
			Input:  m.Rates.Input,
			Output: m.Rates.Output,
		}
	}
	return model
}
