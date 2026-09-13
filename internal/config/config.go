// Package config handles application configuration loading and validation.
package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/routatic/proxy/internal/site"
)

// Config is the root configuration loaded from ~/.config/routatic-proxy/config.json.
// It defines the server settings (host, port), provider connections (OpenCode Go,
// OpenCode Zen, AWS Bedrock, OpenRouter, CommandCode), models, and fallback chains. The config
// supports environment variable interpolation via ${VAR} syntax and hot-reloading
// when hot_reload is enabled.
type Config struct {
	APIKey                         string             `json:"api_key"`
	APIKeys                        []string           `json:"api_keys"`
	Host                           string             `json:"host"`
	Port                           int                `json:"port"`
	HotReload                      bool               `json:"hot_reload"`
	EnableStreamingScenarioRouting bool               `json:"enable_streaming_scenario_routing"`
	EnableCostBasedRouting         bool               `json:"enable_cost_based_routing"`
	CostRouting                    *CostRoutingConfig `json:"cost_routing,omitempty"`
	RespectRequestedModel          *bool              `json:"respect_requested_model,omitempty"`
	// ActiveSite restricts routing to one platform. Empty means unrestricted,
	// which is what an existing config does: setting it is how an operator opts
	// into single-site routing from the dashboard.
	ActiveSite           string                   `json:"active_site,omitempty"`
	Models               map[string]ModelConfig   `json:"models"`
	Fallbacks            map[string][]ModelConfig `json:"fallbacks"`
	ModelOverrides       map[string]ModelConfig   `json:"model_overrides"`
	ModelFamilyOverrides map[string]ModelConfig   `json:"model_family_overrides"`
	AWSBedrock           AWSBedrockConfig         `json:"aws_bedrock"`
	OpenCodeGo           OpenCodeGoConfig         `json:"opencode_go"`
	OpenCodeZen          OpenCodeZenConfig        `json:"opencode_zen"`
	OpenRouter           OpenRouterConfig         `json:"openrouter"`
	CommandCode          CommandCodeConfig        `json:"commandcode"`
	AnthropicFirst       AnthropicFirstConfig     `json:"anthropic_first"`
	Logging              LoggingConfig            `json:"logging"`
	Catalog              CatalogConfig            `json:"catalog"`
	Storage              *StorageConfig           `json:"storage,omitempty"`
}

// CostRoutingConfig controls cost-aware model selection.
type CostRoutingConfig struct {
	Enabled            bool                    `json:"enabled"`
	PreferProviders    []string                `json:"prefer_providers,omitempty"`
	MaxContextWindow   int64                   `json:"max_context_window,omitempty"`
	PenaltyPerProvider map[string]float64      `json:"penalty_per_provider,omitempty"`
	Scenarios          map[string]CostScenario `json:"scenarios,omitempty"`
}

// CostScenarioNames are the routing scenario keys that cost_routing.scenarios
// may define. It mirrors the Scenario constants in internal/router/scenarios.go;
// TestCostScenarioNamesMatchRouterScenarios guards the two against drift.
// "override" is absent on purpose: an explicitly requested model short-circuits
// scenario routing before the cost selector is consulted.
var CostScenarioNames = []string{
	"default",
	"background",
	"think",
	"complex",
	"long_context",
	"fast",
	"vision",
	"vision_complex",
	"vision_long_context",
}

// CostScenario is the per-scenario policy applied on top of the requirements
// already derived from the request itself (token count, tool use, vision,
// reasoning). Every field is additive: a scenario left out of the config uses
// the zero policy, which imposes no extra requirements and lets the cheapest
// model that can serve the request win.
type CostScenario struct {
	// Description is documentation only; it does not affect selection.
	Description string `json:"description,omitempty"`
	// RequiresTools, RequiresVision and RequiresReasoning are hard floors. A
	// nil pointer means "no opinion" — the request's own detected needs still
	// apply.
	RequiresTools     *bool `json:"requires_tools,omitempty"`
	RequiresVision    *bool `json:"requires_vision,omitempty"`
	RequiresReasoning *bool `json:"requires_reasoning,omitempty"`
	// MinContextWindow is a floor in tokens. The effective floor is the larger
	// of this and the request's own token count.
	MinContextWindow int64 `json:"min_context_window,omitempty"`
	// PreferredProviders restricts candidates to these providers. When
	// cost_routing.prefer_providers is also set, the two are intersected.
	PreferredProviders []string `json:"preferred_providers,omitempty"`
}

// CostBasedRoutingEnabled reports whether cost-aware routing should be active.
// It is enabled when either the legacy top-level flag is set or the nested
// cost_routing block explicitly enables it.
func (c *Config) CostBasedRoutingEnabled() bool {
	if c == nil {
		return false
	}
	if c.EnableCostBasedRouting {
		return true
	}
	if c.CostRouting != nil && c.CostRouting.Enabled {
		return true
	}
	return false
}

// AnthropicFirstConfig controls direct Anthropic passthrough with OpenCode fallback.
type AnthropicFirstConfig struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
}

// CatalogConfig controls automatic syncing of the models.dev catalog.
type CatalogConfig struct {
	MaxAgeHours int    `json:"max_age_hours"`
	SourceURL   string `json:"source_url"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

// ModelConfig defines routing rules for a specific model.
type ModelConfig struct {
	Provider               string          `json:"provider"`
	ModelID                string          `json:"model_id"`
	ModelRef               string          `json:"model_ref,omitempty"`
	WireFormat             string          `json:"wire_format,omitempty"` // "auto" (default), "openai", "anthropic", "responses", "gemini"
	Temperature            float64         `json:"temperature"`
	MaxTokens              int             `json:"max_tokens"`
	MaxOutputTokens        int             `json:"max_output_tokens,omitempty"`
	ContextWindow          int             `json:"context_window,omitempty"`
	ContextMargin          int             `json:"context_margin,omitempty"`
	ContextThreshold       int             `json:"context_threshold"`
	SupportsTools          *bool           `json:"supports_tools,omitempty"`
	ReasoningEffort        string          `json:"reasoning_effort"`
	Thinking               json.RawMessage `json:"thinking,omitempty"`
	Vision                 bool            `json:"vision"`
	AnthropicToolsDisabled bool            `json:"anthropic_tools_disabled"`
}

// AWSBedrockConfig holds the upstream AWS Bedrock Mantle API settings.
type AWSBedrockConfig struct {
	BaseURL            string           `json:"base_url"`
	AnthropicBaseURL   string           `json:"anthropic_base_url,omitempty"`
	APIKey             string           `json:"api_key,omitempty"`
	APIKeys            []string         `json:"api_keys,omitempty"`
	ProjectID          string           `json:"project_id,omitempty"`
	WireFormat         string           `json:"wire_format,omitempty"` // "openai" (default), "anthropic"
	TimeoutMs          int              `json:"timeout_ms"`
	StreamTimeoutMs    int              `json:"stream_timeout_ms"`
	StreamingTimeoutMs int              `json:"streaming_timeout_ms,omitempty"`
	Billing            AWSBillingConfig `json:"billing"`
}

// AWSBillingConfig is independent of Bedrock inference keys. The SDK resolves
// its own IAM identity only after an explicitly requested billing query.
type AWSBillingConfig struct {
	Enabled         bool   `json:"enabled"`
	Profile         string `json:"profile,omitempty"`
	LinkedAccountID string `json:"linked_account_id,omitempty"`
}

func (c AWSBillingConfig) Validate() error {
	if c.Enabled || c.LinkedAccountID != "" {
		if len(c.LinkedAccountID) != 12 || strings.IndexFunc(c.LinkedAccountID, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return fmt.Errorf("aws_bedrock.billing.linked_account_id must be a 12-digit account ID; billing queries must have an explicit account scope")
		}
	}
	if envVarPattern.MatchString(c.Profile) {
		return fmt.Errorf("aws_bedrock.billing.profile contains an unresolved environment variable")
	}
	return nil
}

// EffectiveAPIKeys returns the pool of API keys for AWS Bedrock. The APIKeys
// array (for key rotation) takes precedence; if empty, falls back to the single
// APIKey field. This precedence order allows a single key in config with an
// override to multiple keys via environment variable (ROUTATIC_PROXY_AWS_BEDROCK_API_KEYS).
func (c *AWSBedrockConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// OpenCodeGoConfig holds the upstream OpenCode Go API settings.
type OpenCodeGoConfig struct {
	BaseURL            string   `json:"base_url"`
	AnthropicBaseURL   string   `json:"anthropic_base_url"`
	ResponsesBaseURL   string   `json:"responses_base_url"`
	APIKey             string   `json:"api_key,omitempty"`
	APIKeys            []string `json:"api_keys,omitempty"`
	TimeoutMs          int      `json:"timeout_ms"`
	StreamTimeoutMs    int      `json:"stream_timeout_ms"`
	StreamingTimeoutMs int      `json:"streaming_timeout_ms,omitempty"`
}

// EffectiveAPIKeys returns the pool of API keys for OpenCode Go. The APIKeys
// array (for key rotation) takes precedence; if empty, falls back to the single
// APIKey field. This precedence order allows a single key in config with an
// override to multiple keys via environment variable (ROUTATIC_PROXY_OPENCODE_GO_API_KEYS).
func (c *OpenCodeGoConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// OpenRouterConfig holds the upstream OpenRouter API settings.
type OpenRouterConfig struct {
	BaseURL            string   `json:"base_url"`
	APIKey             string   `json:"api_key,omitempty"`
	APIKeys            []string `json:"api_keys,omitempty"`
	ManagementAPIKey   string   `json:"management_api_key,omitempty"` // Account credits only; never used for inference.
	TimeoutMs          int      `json:"timeout_ms"`
	StreamTimeoutMs    int      `json:"stream_timeout_ms"`
	StreamingTimeoutMs int      `json:"streaming_timeout_ms,omitempty"`
}

// EffectiveAPIKeys returns the pool of API keys for OpenRouter.
// APIKeys takes precedence; falls back to the single APIKey field.
func (c *OpenRouterConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// CommandCodeConfig uses the official Provider API, with its own credentials.
// BaseURL and AnthropicBaseURL are complete endpoint URLs, like OpenCode Go.
type CommandCodeConfig struct {
	BaseURL            string   `json:"base_url"`
	AnthropicBaseURL   string   `json:"anthropic_base_url"`
	APIKey             string   `json:"api_key,omitempty"`
	APIKeys            []string `json:"api_keys,omitempty"`
	TimeoutMs          int      `json:"timeout_ms"`
	StreamTimeoutMs    int      `json:"stream_timeout_ms"`
	StreamingTimeoutMs int      `json:"streaming_timeout_ms,omitempty"`
	ZeroDataRetention  bool     `json:"zero_data_retention"`
}

func (c *CommandCodeConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// OpenCodeZenConfig holds the upstream OpenCode Zen API settings.
type OpenCodeZenConfig struct {
	BaseURL            string   `json:"base_url"`
	AnthropicBaseURL   string   `json:"anthropic_base_url"`
	ResponsesBaseURL   string   `json:"responses_base_url"`
	GeminiBaseURL      string   `json:"gemini_base_url"`
	APIKey             string   `json:"api_key,omitempty"`
	APIKeys            []string `json:"api_keys,omitempty"`
	TimeoutMs          int      `json:"timeout_ms"`
	StreamTimeoutMs    int      `json:"stream_timeout_ms"`
	StreamingTimeoutMs int      `json:"streaming_timeout_ms,omitempty"`
}

// EffectiveAPIKeys returns the pool of API keys for OpenCode Zen. The APIKeys
// array (for key rotation) takes precedence; if empty, falls back to the single
// APIKey field. This precedence order allows a single key in config with an
// override to multiple keys via environment variable (ROUTATIC_PROXY_OPENCODE_ZEN_API_KEYS).
func (c *OpenCodeZenConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// LoggingConfig controls application logging behavior.
type LoggingConfig struct {
	Level        string        `json:"level"`
	Requests     bool          `json:"requests"`
	DebugCapture *DebugCapture `json:"debug_capture,omitempty"`
}

// StorageConfig controls persistent storage settings.
type StorageConfig struct {
	DatabasePath    string `json:"database_path"`
	RetentionDays   int    `json:"retention_days"` // Negative disables cleanup; zero or omitted keeps 7 days.
	VacuumOnStartup bool   `json:"vacuum_on_startup"`

	// WALEnabled is a pointer so an explicit false stays distinguishable from
	// an absent field. The default is true, and with a plain bool there would
	// be no way to turn WAL off from config: the zero value and "unset" would
	// look identical to the overlay below.
	WALEnabled *bool `json:"wal_enabled,omitempty"`

	// AnalyticsBaseline (RFC3339) makes the analytics tab ignore requests older
	// than this instant. Rows written before the proxy learned to read the
	// upstream cache fields booked an entire prompt as fresh input, which
	// overstates cost by up to 50x; the hit/miss split is absent from those
	// rows, so they cannot be repaired and are excluded instead. Empty means
	// analyse the full history.
	AnalyticsBaseline string `json:"analytics_baseline,omitempty"`
}

// DebugCapture controls request/response capture for debugging.
type DebugCapture struct {
	Enabled       bool   `json:"enabled"`
	Directory     string `json:"directory"`
	MaxFiles      int    `json:"max_files"`
	MaxFileSize   int64  `json:"max_file_size"`
	RedactAPIKeys bool   `json:"redact_api_keys"`
}

// EffectiveAPIKeys returns the global pool of API keys for rotation. The APIKeys
// array takes precedence; if empty, falls back to the single APIKey field. This
// precedence order is used by providers that lack their own key configuration,
// falling back to global keys when neither provider-specific keys nor environment
// overrides are set.
func (c *Config) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// NormalizeProvider preserves the legacy empty-provider default and spelling.
func NormalizeProvider(provider string) string {
	return site.Normalize(provider)
}

// ModelKey identifies a routing target, including its provider.
func ModelKey(model ModelConfig) string {
	return NormalizeProvider(model.Provider) + "/" + model.ModelID
}

// SupportedProvider reports whether the runtime has an adapter for this name.
func SupportedProvider(provider string) bool {
	return site.IsKnown(provider)
}

// providerKeySource binds each platform to the config fields holding its own
// credentials. Config knows its own shape and site knows which platforms exist;
// keeping the binding here rather than in the registry is what avoids the
// import cycle, and providerKeySourceCoversRegistry keeps the two lists from
// drifting apart.
//
// globalFallback repeats the long-standing rule that the four original
// platforms may use the global key. CommandCode must not: its credential is not
// an OpenCode one, and a global key issued for another host would be sent to a
// host it was never issued for.
var providerKeySource = map[string]struct {
	keys           func(*Config) []string
	globalFallback bool
}{
	site.OpenCodeGo:  {func(c *Config) []string { return c.OpenCodeGo.EffectiveAPIKeys() }, true},
	site.OpenCodeZen: {func(c *Config) []string { return c.OpenCodeZen.EffectiveAPIKeys() }, true},
	site.AWSBedrock:  {func(c *Config) []string { return c.AWSBedrock.EffectiveAPIKeys() }, true},
	site.OpenRouter:  {func(c *Config) []string { return c.OpenRouter.EffectiveAPIKeys() }, true},
	site.CommandCode: {func(c *Config) []string { return c.CommandCode.EffectiveAPIKeys() }, false},
}

// ProviderModelEndpoint is the configured API endpoint a platform's model list
// is derived from. Empty means this platform has no model-list endpoint of its
// own, so its models come from the shared catalog instead.
func (c *Config) ProviderModelEndpoint(provider string) string {
	switch site.Normalize(provider) {
	case site.CommandCode:
		return c.CommandCode.BaseURL
	default:
		return ""
	}
}

// ProviderAPIKeys is the credential source shared by sending and fallback logic.
// A platform with no key of its own falls back to the global key only when the
// binding above allows it; unknown providers must never receive another
// platform's credentials.
func (c *Config) ProviderAPIKeys(provider string) []string {
	source, ok := providerKeySource[site.Normalize(provider)]
	if !ok {
		return nil
	}
	if keys := source.keys(c); len(keys) > 0 {
		return keys
	}
	if !source.globalFallback {
		return nil
	}
	return c.EffectiveAPIKeys()
}

// ProviderTimeouts is one platform's configured timeouts, in milliseconds, with
// the idle and total values already fallen back to the platform's overall
// timeout. Zero means unset everywhere, which callers read as "use their own
// default".
type ProviderTimeouts struct {
	RequestMs        int
	StreamIdleMs     int
	StreamingTotalMs int
}

// providerTimeoutSource binds each platform to the config block holding its
// timeouts, the same way providerKeySource binds its credentials. A platform
// with no block of its own - one served by the generic OpenAI-compatible path -
// uses the default platform's block, which is what the per-platform switches
// this replaces did.
var providerTimeoutSource = map[string]func(*Config) (request, idle, total int){
	site.OpenCodeGo: func(c *Config) (int, int, int) {
		return c.OpenCodeGo.TimeoutMs, c.OpenCodeGo.StreamTimeoutMs, c.OpenCodeGo.StreamingTimeoutMs
	},
	site.OpenCodeZen: func(c *Config) (int, int, int) {
		return c.OpenCodeZen.TimeoutMs, c.OpenCodeZen.StreamTimeoutMs, c.OpenCodeZen.StreamingTimeoutMs
	},
	site.AWSBedrock: func(c *Config) (int, int, int) {
		return c.AWSBedrock.TimeoutMs, c.AWSBedrock.StreamTimeoutMs, c.AWSBedrock.StreamingTimeoutMs
	},
	site.OpenRouter: func(c *Config) (int, int, int) {
		return c.OpenRouter.TimeoutMs, c.OpenRouter.StreamTimeoutMs, c.OpenRouter.StreamingTimeoutMs
	},
	site.CommandCode: func(c *Config) (int, int, int) {
		return c.CommandCode.TimeoutMs, c.CommandCode.StreamTimeoutMs, c.CommandCode.StreamingTimeoutMs
	},
}

// ProviderTimeouts returns a platform's configured timeouts.
func (c *Config) ProviderTimeouts(provider string) ProviderTimeouts {
	source, ok := providerTimeoutSource[site.Normalize(provider)]
	if !ok {
		source = providerTimeoutSource[site.DefaultID()]
	}
	request, idle, total := source(c)
	if idle <= 0 {
		idle = request
	}
	if total <= 0 {
		total = request
	}
	return ProviderTimeouts{RequestMs: request, StreamIdleMs: idle, StreamingTotalMs: total}
}
