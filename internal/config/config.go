// Package config handles application configuration loading and validation.
package config

import "encoding/json"

// Config holds the complete application configuration.
type Config struct {
	APIKey                         string                   `json:"api_key"`
	APIKeys                        []string                 `json:"api_keys"`
	Host                           string                   `json:"host"`
	Port                           int                      `json:"port"`
	HotReload                      bool                     `json:"hot_reload"`
	EnableStreamingScenarioRouting bool                     `json:"enable_streaming_scenario_routing"`
	RespectRequestedModel          bool                     `json:"respect_requested_model"`
	Models                         map[string]ModelConfig   `json:"models"`
	Fallbacks                      map[string][]ModelConfig `json:"fallbacks"`
	ModelOverrides                 map[string]ModelConfig   `json:"model_overrides"`
	OpenCodeGo                     OpenCodeGoConfig         `json:"opencode_go"`
	OpenCodeZen                    OpenCodeZenConfig        `json:"opencode_zen"`
	Logging                        LoggingConfig            `json:"logging"`
}

// ModelConfig defines routing rules for a specific model.
type ModelConfig struct {
	Provider         string          `json:"provider"`
	ModelID          string          `json:"model_id"`
	Temperature      float64         `json:"temperature"`
	MaxTokens        int             `json:"max_tokens"`
	ContextThreshold int             `json:"context_threshold"`
	ReasoningEffort  string          `json:"reasoning_effort"`
	Thinking         json.RawMessage `json:"thinking,omitempty"`
	Vision           bool            `json:"vision"`
	// ContextWindow is the model's context-window size in tokens. ≤ 0 means
	// "unknown / unconstrained", which disables capacity filtering for the
	// model. When set, requests whose context usage leaves fewer than
	// ContextMargin (or the minimum output floor) tokens are filtered out.
	ContextWindow int `json:"context_window"`
	// ContextMargin is reserved headroom subtracted from the context window
	// when computing how many output tokens a request may produce.
	ContextMargin int `json:"context_margin"`
	// MaxOutputTokens is a model-level cap on output tokens, applied after
	// the client's max_tokens. ≤ 0 means no model-level cap.
	MaxOutputTokens int `json:"max_output_tokens"`
}

// OpenCodeGoConfig holds the upstream OpenCode Go API settings.
type OpenCodeGoConfig struct {
	BaseURL          string `json:"base_url"`
	AnthropicBaseURL string `json:"anthropic_base_url"`
	TimeoutMs        int    `json:"timeout_ms"`
}

// OpenCodeZenConfig holds the upstream OpenCode Zen API settings.
type OpenCodeZenConfig struct {
	BaseURL          string `json:"base_url"`
	AnthropicBaseURL string `json:"anthropic_base_url"`
	ResponsesBaseURL string `json:"responses_base_url"`
	GeminiBaseURL    string `json:"gemini_base_url"`
	TimeoutMs        int    `json:"timeout_ms"`
}

// LoggingConfig controls application logging behavior.
type LoggingConfig struct {
	Level    string `json:"level"`
	Requests bool   `json:"requests"`
}

// EffectiveAPIKeys returns the pool of API keys for rotation.
// APIKeys takes precedence; falls back to the single APIKey field.
func (c *Config) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}
