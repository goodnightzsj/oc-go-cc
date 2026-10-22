package catalog

// providerModelOverrides adds models a platform serves but models.dev does not
// publish under it. Applied on every load, so a sync cannot drop them: the
// import replaces the whole models table from this catalog, and the mirror is
// the only other contributor.
//
// This exists because the mirror files a model under its vendor while the
// platform lists it under its own namespace. Cline serves
// "cline-pass/deepseek-v4.1-flash-1017" and answers images sent to it, but
// models.dev records that model only as "deepseek/deepseek-v4.1-flash-1017", so
// the platform's own name resolved to no capabilities at all. The lookup side is
// fixed too - see router.catalogResolved - and this table is for the case where
// no entry exists under any namespace, or where the platform's own numbers are
// the ones that should apply.
//
// An entry already published by the mirror is never replaced: the mirror is the
// authority for models it knows, and silently preferring a hand-written copy
// would let this table go stale without anything noticing.
//
// Prices are deliberately absent from this table. The platform's rates live in
// the embedded table and its hourly refresher, which is what the cost column
// and the dashboard both read; a rate written here would be a second copy that
// nobody updates when the platform changes its prices. Only the capability
// facts this table exists to supply - context window, output ceiling, vision,
// tool support - are stated.
var providerModelOverrides = map[string]map[string]Model{
	"cline-pass": {
		// Verified against the platform: a 256x256 red PNG answers "Red", and
		// the gateway reports canonicalSlug deepseek/deepseek-v4.1-flash-1017.
		// Limits are the ones the platform publishes for its sibling
		// deepseek-v4.1-flash; the mirror's numbers for the vendor entry agree.
		"deepseek-v4.1-flash-1017": {
			ID:        "cline-pass/deepseek-v4.1-flash-1017",
			Name:      "DeepSeek V4.1 Flash 1017",
			Reasoning: true,
			ToolCall:  true,
			Modalities: Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Limit: &Limit{Context: 1000000, Output: 384000},
		},
	},
}

// applyProviderOverrides adds the table's entries to a loaded catalog.
//
// It also registers them in the provider's own nested map. Load populates that
// map from the mirror and ProviderModels is built from the merged top level, so
// skipping it would leave the entry resolvable by name but absent from the
// provider's own listing.
func applyProviderOverrides(catalog *Catalog) {
	for providerName, models := range providerModelOverrides {
		provider, ok := catalog.Providers[providerName]
		if !ok {
			// A provider the catalog does not carry cannot be addressed anyway;
			// adding it would mean inventing a base URL.
			continue
		}
		if provider.Models == nil {
			provider.Models = make(map[string]Model)
		}
		for name, model := range models {
			key := providerName + "/" + name
			// The mirror wins wherever it has an opinion.
			if _, exists := catalog.Models[key]; exists {
				if _, nested := provider.Models[name]; !nested {
					provider.Models[name] = catalog.Models[key]
				}
				continue
			}
			if model.ID == "" {
				model.ID = key
			}
			catalog.Models[key] = model
			provider.Models[name] = model
		}
		catalog.Providers[providerName] = provider
	}
}
