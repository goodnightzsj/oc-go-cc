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
	// Overrides carries the conditional prices models.dev publishes alongside
	// the base rate. It is not read from models.dev today - that source omits
	// the field entirely - but the shape is stated here because OpenRouter's own
	// endpoint is where it comes from, and because parsing it is what keeps a
	// time-window override from being mistaken for a tier.
	Overrides []CostOverride `json:"overrides,omitempty"`
}

// CostOverride is one conditional price from a `cost.overrides` array.
//
// Two kinds arrive, distinguished by which condition is set: a long-context
// tier (`MinPromptTokens`) and a time window (`UTCDays`, `UTCStart`, `UTCEnd`).
// Only the first is applied to a price; see Tiers for why the second is not.
type CostOverride struct {
	// MinPromptTokens conditions the override on prompt size, making it a tier.
	MinPromptTokens int64 `json:"min_prompt_tokens,omitempty"`

	// The time-window form. Parsed rather than ignored, because the alternative
	// is worse than doing nothing: an override whose condition goes unread
	// becomes an unconditional one, and every request is billed at whichever
	// band happened to be listed.
	UTCDays  []string `json:"utc_days,omitempty"`
	UTCStart *int     `json:"utc_start,omitempty"`
	UTCEnd   *int     `json:"utc_end,omitempty"`

	Prompt         float64 `json:"prompt,omitempty"`
	Completion     float64 `json:"completion,omitempty"`
	InputCacheRead float64 `json:"input_cache_read,omitempty"`
}

// IsTier reports whether this override is a prompt-size threshold rather than a
// time window. An override carrying neither condition is not a tier: applying
// it unconditionally would replace every request's price.
func (o CostOverride) IsTier() bool {
	return o.MinPromptTokens > 0 && !o.IsTimeWindow()
}

// IsTimeWindow reports whether this override is conditioned on the clock.
func (o CostOverride) IsTimeWindow() bool {
	return len(o.UTCDays) > 0 || o.UTCStart != nil || o.UTCEnd != nil
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

// RatesAt returns the rates that apply to a request of the given prompt size,
// falling back to the base rates when no tier covers it.
//
// A tier applies strictly above its threshold, which is how models.dev words it
// ("Condition: applies when total prompt tokens are strictly greater than this
// threshold"), and the highest applicable threshold wins. Applying the wrong
// direction here is a factor-of-two error on exactly the largest requests, the
// ones a tier exists to price.
//
// Time-window overrides are skipped rather than applied. models.dev does not
// publish them, OpenRouter's own two time-windowed models express windows that
// history.peakSchedules cannot represent (per-model multipliers, and one running
// in the opposite direction from DeepSeek's), and guessing a window from a
// payload that also carries weekday names would be inventing a rule. Skipping
// leaves such a model priced at its listed base rate, which is the cheaper band
// for both known cases - under-billing, stated in docs/openrouter.md, rather
// than a coin flip.
func (m Model) RatesAt(promptTokens int64) *Rates {
	if m.Cost == nil {
		return nil
	}
	best := int64(-1)
	var chosen *CostOverride
	for i := range m.Cost.Overrides {
		o := &m.Cost.Overrides[i]
		if !o.IsTier() {
			continue
		}
		if promptTokens > o.MinPromptTokens && o.MinPromptTokens > best {
			best, chosen = o.MinPromptTokens, o
		}
	}
	if chosen == nil {
		return m.Rates()
	}
	// Each field falls back to the base independently: an override that omits a
	// cache rate means "unchanged", not "free".
	out := &Rates{Input: chosen.Prompt, Output: chosen.Completion}
	if out.Input == 0 {
		out.Input = m.Cost.Input
	}
	if out.Output == 0 {
		out.Output = m.Cost.Output
	}
	return out
}

// PromptTiers returns the model's long-context thresholds in ascending order,
// for storage and for reporting which bands a model has.
func (m Model) PromptTiers() []PromptTier {
	if m.Cost == nil {
		return nil
	}
	out := make([]PromptTier, 0, len(m.Cost.Overrides))
	for _, o := range m.Cost.Overrides {
		if !o.IsTier() {
			continue
		}
		t := PromptTier{MinPromptTokens: o.MinPromptTokens, Input: o.Prompt, Output: o.Completion}
		if o.InputCacheRead != 0 {
			t.CacheRead = o.InputCacheRead
		}
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b PromptTier) int {
		return int(a.MinPromptTokens - b.MinPromptTokens)
	})
	return out
}

// PromptTier is one long-context price band, in this package's own vocabulary.
// Named for the condition rather than "tier" so a caller cannot confuse it with
// a time window, which is the other thing the same array carries.
type PromptTier struct {
	// MinPromptTokens is the threshold the band applies strictly above.
	MinPromptTokens int64   `json:"min_prompt_tokens"`
	Input           float64 `json:"input"`
	Output          float64 `json:"output"`
	CacheRead       float64 `json:"cache_read,omitempty"`
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
