// Package router defines HTTP route registration and middleware chaining,
// as well as model selection based on request scenarios.
package router

import (
	"fmt"

	"oc-go-cc/internal/config"
)

// ModelRouter handles model selection based on scenarios.
type ModelRouter struct {
	atomic *config.AtomicConfig
}

// NewModelRouter creates a new model router.
func NewModelRouter(atomic *config.AtomicConfig) *ModelRouter {
	return &ModelRouter{atomic: atomic}
}

// RouteResult contains the selected model and fallback chain.
type RouteResult struct {
	Primary   config.ModelConfig
	Fallbacks []config.ModelConfig
	Scenario  Scenario
}

// Route determines which model to use for a request.
// If respect_requested_model is enabled and requestedModel is provided, it overrides scenario-based routing.
func (r *ModelRouter) Route(messages []MessageContent, tokenCount int, requestedModel string) (RouteResult, error) {
	cfg := r.atomic.Get()

	// If configured to respect user's model choice and user specified a model, use it directly
	if cfg.RespectRequestedModel && requestedModel != "" {
		// Create model config from the requested model
		primary := config.ModelConfig{
			Provider: "opencode-go",
			ModelID:  requestedModel,
		}

		// Get default fallbacks
		fallbacks := cfg.Fallbacks["default"]

		return RouteResult{
			Primary:   primary,
			Fallbacks: fallbacks,
			Scenario:  ScenarioDefault,
		}, nil
	}

	// Otherwise, use scenario-based routing
	result := DetectScenario(messages, tokenCount, cfg)

	// Get primary model for scenario
	primary, ok := cfg.Models[string(result.Scenario)]
	if !ok {
		// Fall back to default if scenario model not configured
		primary, ok = cfg.Models["default"]
		if !ok {
			return RouteResult{}, fmt.Errorf("no default model configured")
		}
	}

	// Get fallbacks for scenario
	fallbacks := cfg.Fallbacks[string(result.Scenario)]
	if len(fallbacks) == 0 {
		// Fall back to default fallbacks
		fallbacks = cfg.Fallbacks["default"]
	}

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  result.Scenario,
	}, nil
}

// IsStreamingScenarioRoutingEnabled returns whether streaming requests should use
// scenario-based routing instead of always routing to the fast model.
func (r *ModelRouter) IsStreamingScenarioRoutingEnabled() bool {
	return r.atomic.Get().EnableStreamingScenarioRouting
}

// GetModelChain returns the full chain of models to try (primary + fallbacks).
func (rr *RouteResult) GetModelChain() []config.ModelConfig {
	chain := []config.ModelConfig{rr.Primary}
	chain = append(chain, rr.Fallbacks...)
	return chain
}

// RouteForStreaming determines which model to use for streaming requests.
// Prioritizes fast TTFT (time-to-first-token) over capability.
// If respect_requested_model is enabled and requestedModel is provided, it overrides scenario-based routing.
func (r *ModelRouter) RouteForStreaming(messages []MessageContent, tokenCount int, requestedModel string) RouteResult {
	cfg := r.atomic.Get()

	// If configured to respect user's model choice and user specified a model, use it directly
	if cfg.RespectRequestedModel && requestedModel != "" {
		primary := config.ModelConfig{
			Provider: "opencode-go",
			ModelID:  requestedModel,
		}
		fallbacks := cfg.Fallbacks["default"]

		return RouteResult{
			Primary:   primary,
			Fallbacks: fallbacks,
			Scenario:  ScenarioDefault,
		}
	}

	// Otherwise, use scenario-based routing for streaming
	result := RouteForStreaming(messages, tokenCount, cfg)

	// Get primary model for scenario
	primary, ok := cfg.Models[string(result.Scenario)]
	if !ok {
		// Fall back to fast scenario if not configured
		primary, ok = cfg.Models["fast"]
		if !ok {
			// Fall back to default
			primary = cfg.Models["default"]
		}
	}

	// Get fallbacks for scenario
	fallbacks := cfg.Fallbacks[string(result.Scenario)]
	if len(fallbacks) == 0 {
		// Fall back to fast fallbacks
		fallbacks = cfg.Fallbacks["fast"]
	}

	return RouteResult{
		Primary:   primary,
		Fallbacks: fallbacks,
		Scenario:  result.Scenario,
	}
}
