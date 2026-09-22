package catalog

import (
	"slices"
	"strings"
)

// Catalog is the parsed contents of a models.dev catalog.
type Catalog struct {
	Providers map[string]Provider `json:"providers"`
	Models    map[string]Model    `json:"models"`
}

// Provider describes a model hosting endpoint.
type Provider struct {
	Name                   string `json:"name"`
	BaseURL                string `json:"base_url"`
	APIKey                 string `json:"api_key"`
	Enabled                *bool  `json:"enabled,omitempty"`
	AnthropicToolsDisabled bool   `json:"anthropic_tools_disabled"`

	// Models is models.dev's per-provider model map. The catalog carries two
	// views of the same data: a top-level "models" map keyed by the full
	// provider/model id, and this nested one keyed by the bare name. Neither is
	// complete - a provider added after the top-level map was built (cline-pass
	// among them) appears only here - so the loader reads both.
	Models map[string]Model `json:"models,omitempty"`
}

// Modalities describes the input/output formats a model supports.
type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// Limit describes model usage limits.
type Limit struct {
	Context int64 `json:"context"`
	Output  int64 `json:"output"`
}

// Rates describes model pricing per million tokens.
//
// This is the proxy's own shape, not models.dev's. models.dev publishes rates
// under "cost" (and only input/output; cache rates live in the per-model
// payload too but are not read here). Distinct types rather than one tag
// because the two names mean different things: "rates" is what this proxy
// prices with, "cost" is what the catalog happens to call it, and the mapping
// between them is stated once, below.
type Rates struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// Cost is models.dev's own pricing object, exported because it is part of the
// parsed catalog contract: a caller building a Model by hand must be able to
// state a price the same way the JSON does. It is a separate type so the
// upstream field name appears exactly once, at the conversion, instead of being
// renamed into this package's vocabulary and then silently not matching.
//
// This existed as a live bug: Model.Rates carried the tag `rates`, the catalog
// publishes `cost`, and the mismatch is invisible - Rates stays nil, the model
// is stored with no price, and every cost reads as unknown rather than as
// wrong. Three platforms (OpenCode Go, CommandCode, ClinePass) hid it, because
// each has a hand-written seed table that fills the price in regardless. Any
// platform priced only from the catalog got nothing.
type Cost struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// Model describes a model available through one or more providers.
// The provider is encoded in the model key (e.g. "xai/grok-4.5").
type Model struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Reasoning  bool       `json:"reasoning"`
	ToolCall   bool       `json:"tool_call"`
	Modalities Modalities `json:"modalities"`
	Limit      *Limit     `json:"limit,omitempty"`
	Cost       *Cost      `json:"cost,omitempty"`
}

// Rates returns the model's published per-million-token rates, or nil when the
// catalog carries none. A model with no rates is a different thing from one
// costing zero, and the callers below depend on that distinction.
func (m Model) Rates() *Rates {
	if m.Cost == nil {
		return nil
	}
	return &Rates{Input: m.Cost.Input, Output: m.Cost.Output}
}

// DisplayName returns the model's display name.
func (m Model) DisplayName() string {
	return m.Name
}

// SupportsTools returns whether the model supports tool calls.
func (m Model) SupportsTools() bool {
	return m.ToolCall
}

// SupportsVision returns whether the model supports image inputs.
func (m Model) SupportsVision() bool {
	return slices.Contains(m.Modalities.Input, "image")
}

// ContextWindow returns the model's context window limit, or 0 if unknown.
func (m Model) ContextWindow() int64 {
	if m.Limit != nil {
		return m.Limit.Context
	}
	return 0
}

// MaxOutputTokens returns the model's published output ceiling, or 0 if
// unknown. The zero is "not recorded", never "may not produce output": callers
// treat it as no ceiling rather than as a cap of nothing.
func (m Model) MaxOutputTokens() int64 {
	if m.Limit != nil {
		return m.Limit.Output
	}
	return 0
}

// CostInputPerM returns the input cost per million tokens, or 0 if unknown.
func (m Model) CostInputPerM() float64 {
	if r := m.Rates(); r != nil {
		return r.Input
	}
	return 0
}

// CostOutputPerM returns the output cost per million tokens, or 0 if unknown.
func (m Model) CostOutputPerM() float64 {
	if r := m.Rates(); r != nil {
		return r.Output
	}
	return 0
}

// ProviderFromModelKey extracts the provider name from a model key
// of the form "provider/model-name". Returns "" if no separator found.
func ProviderFromModelKey(key string) string {
	idx := strings.IndexByte(key, '/')
	if idx < 0 {
		return ""
	}
	return key[:idx]
}

// ModelNameFromKey extracts the model name portion from a model key
// of the form "provider/model-name". Returns the full key if no separator.
func ModelNameFromKey(key string) string {
	idx := strings.IndexByte(key, '/')
	if idx < 0 {
		return key
	}
	return key[idx+1:]
}

// modelNameFromKey is an alias for internal use.
func modelNameFromKey(key string) string {
	return ModelNameFromKey(key)
}

// Selector is a parsed model reference such as model@provider,
// lab/model@provider, or a short id.
type Selector struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Alias    string `json:"alias"`
}

// ResolvedModel is a fully materialized provider/model pair ready for use.
type ResolvedModel struct {
	Provider               string  `json:"provider"`
	ModelID                string  `json:"model_id"`
	CanonicalName          string  `json:"canonical_name"`
	DisplayName            string  `json:"display_name"`
	BaseURL                string  `json:"base_url"`
	APIKey                 string  `json:"api_key"`
	AnthropicToolsDisabled bool    `json:"anthropic_tools_disabled"`
	ContextWindow          int64   `json:"context_window"`
	MaxOutputTokens        int64   `json:"max_output_tokens"`
	CostInputPerM          float64 `json:"cost_input_per_m"`
	CostOutputPerM         float64 `json:"cost_output_per_m"`
	Tools                  bool    `json:"tools"`
	Vision                 bool    `json:"vision"`
	Reasoning              bool    `json:"reasoning"`
}
