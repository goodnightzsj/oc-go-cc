package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// IndexedCatalog is a Catalog with an additional index from provider name to
// the models that declare support for that provider.
type IndexedCatalog struct {
	Catalog
	ProviderModels map[string][]Model
}

// ModelsForProvider returns the models that support the named provider.
// It returns nil when the provider has no indexed models.
func (ic *IndexedCatalog) ModelsForProvider(provider string) []Model {
	return ic.ProviderModels[provider]
}

// Load reads a catalog from path, validates its contents, and returns an
// indexed view.
func Load(path string) (*IndexedCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read catalog file: %w", err)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse catalog json: %w", err)
	}

	if err := validateCatalog(&catalog); err != nil {
		return nil, err
	}

	// Fold each provider's nested model map into the top-level one. The catalog
	// holds the same models twice and neither view is complete: models.dev's
	// per-provider maps are authoritative for that provider's roster, while the
	// top-level map is what carries ids for providers it has already indexed.
	// Reading only the top level drops any provider it does not cover, which is
	// how cline-pass's capabilities went missing while its models still listed.
	mergeProviderModels(&catalog)

	idx := &IndexedCatalog{
		Catalog:        catalog,
		ProviderModels: make(map[string][]Model, len(catalog.Providers)),
	}

	for key, model := range catalog.Models {
		provider := ProviderFromModelKey(key)
		if provider == "" {
			continue
		}
		// Stamp the key onto the model. Callers read Model.ID to learn which
		// model this is, and an entry folded in from a provider's nested map
		// carries no id of its own - leaving it blank made those models
		// unaddressable, silently, for every provider that publishes that way.
		if model.ID == "" {
			model.ID = key
		}
		idx.ProviderModels[provider] = append(idx.ProviderModels[provider], model)
	}

	return idx, nil
}

// mergeProviderModels copies every provider-nested model into the top-level map
// under its provider-qualified key, keeping whatever the top level already had
// for that key. The top level wins because it is the view other code already
// resolves against; the nested entry only fills ids it does not carry.
//
// The nested key is not always a bare model name. models.dev qualifies it for
// some providers and not others - 15 of 232 do, including every cline-pass
// entry, where the key is already "cline-pass/deepseek-v4-flash" - so prefixing
// unconditionally produces "cline-pass/cline-pass/deepseek-v4-flash". That
// double-prefixed key is not inert: it is a second, wrong entry in the same
// listing, and a client that picked it would send the doubled string upstream.
// A key that already carries its provider is therefore kept as it is.
func mergeProviderModels(catalog *Catalog) {
	if catalog.Models == nil {
		catalog.Models = make(map[string]Model)
	}
	for providerName, provider := range catalog.Providers {
		for name, model := range provider.Models {
			key := name
			if !strings.HasPrefix(name, providerName+"/") {
				key = providerName + "/" + name
			}
			if _, exists := catalog.Models[key]; exists {
				continue
			}
			if model.ID == "" {
				model.ID = key
			}
			catalog.Models[key] = model
		}
	}
}

func validateCatalog(catalog *Catalog) error {
	if len(catalog.Providers) == 0 {
		return errors.New("catalog providers map is empty")
	}
	if len(catalog.Models) == 0 {
		return errors.New("catalog models map is empty")
	}

	var toDelete []string
	for key := range catalog.Models {
		provider := ProviderFromModelKey(key)
		if provider == "" {
			return fmt.Errorf("model key %q does not include a provider prefix (expected format: provider/model)", key)
		}
		if _, ok := catalog.Providers[provider]; !ok {
			slog.Warn("skipping model with unknown provider", "model", key, "provider", provider)
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		delete(catalog.Models, key)
	}

	if len(toDelete) > 0 && len(catalog.Models) == 0 {
		return errors.New("all models reference unknown providers, catalog is unusable")
	}

	return nil
}
