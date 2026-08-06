package config

// DefaultContextMargin is the default headroom (in tokens) reserved inside a
// model's context window for the response. It is applied to every model unless
// a ModelConfig explicitly sets ContextMargin.
const DefaultContextMargin = 8192

// ModelMetadata describes a known model's capacity capabilities. This metadata
// lets ResolveModelConfig fill in ContextWindow / MaxOutputTokens / Vision
// defaults for models the user's runtime config lists without these optional
// fields, so capacity filtering works without spelling every value out.
type ModelMetadata struct {
	ContextWindow   int
	MaxOutputTokens int
	Vision          bool
}

// modelMetadata maps a model ID to its built-in capacity capacity. Only models
// listed here participate in capacity filtering unless the config overrides it.
var modelMetadata = map[string]ModelMetadata{
	"deepseek-v4-pro":        {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: false},
	"deepseek-v4-flash":      {ContextWindow: 1000000, MaxOutputTokens: 4096, Vision: false},
	"deepseek-v4-flash-free": {ContextWindow: 1000000, MaxOutputTokens: 4096, Vision: false},
	"glm-5.2":                {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false},
	"glm-5.1":                {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false},
	"glm-5":                  {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false},
	"kimi-k3":                {ContextWindow: 1000000, MaxOutputTokens: 131072, Vision: true},
	"kimi-k2.7-code":         {ContextWindow: 256000, MaxOutputTokens: 32768, Vision: true},
	"kimi-k2.6":              {ContextWindow: 256000, MaxOutputTokens: 8192, Vision: true},
	"kimi-k2.5":              {ContextWindow: 256000, MaxOutputTokens: 8192, Vision: true},
	"mimo-v2-omni":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true},
	"mimo-v2.5-pro":          {ContextWindow: 1000000, MaxOutputTokens: 16384, Vision: false},
	"mimo-v2.5":              {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: false},
	"mimo-v2-pro":            {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: false},
	"minimax-m3":             {ContextWindow: 1000000, MaxOutputTokens: 128000, Vision: false},
	"minimax-m2.7":           {ContextWindow: 200000, MaxOutputTokens: 8192, Vision: false},
	"minimax-m2.5":           {ContextWindow: 200000, MaxOutputTokens: 4096, Vision: false},
	"qwen3.7-max":            {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true},
	"qwen3.7-plus":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true},
	"qwen3.6-plus":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true},
	"mimo-v2.5-free":         {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: false},
	"qwen3.5-plus":           {ContextWindow: 1000000, MaxOutputTokens: 8192, Vision: true},
}

// ResolveModelConfig fills in default capacity values (ContextWindow,
// MaxOutputTokens, Vision) for a ModelConfig by consulting the built-in
// modelMetadata registry. Existing non-zero / explicitly-set values are
// preserved. Models not in the registry keep their declared values (typically
// zero), which capacity filtering treats as "unconstrained".
//
// Call this before capacity filtering so per-model context limits are accurate.
// ContextMargin defaults to DefaultContextMargin unless already set.
func ResolveModelConfig(model ModelConfig) ModelConfig {
	if meta, ok := modelMetadata[model.ModelID]; ok {
		if model.ContextWindow == 0 {
			model.ContextWindow = meta.ContextWindow
		}
		if model.MaxOutputTokens == 0 {
			model.MaxOutputTokens = meta.MaxOutputTokens
		}
		if !model.Vision {
			model.Vision = meta.Vision
		}
	}
	if model.ContextMargin == 0 {
		model.ContextMargin = DefaultContextMargin
	}
	return model
}
