package router

import (
	"fmt"

	"oc-go-cc/internal/config"
)

// minimumOutputTokens is the smallest output budget a model must retain in its
// context window to be considered eligible for a request.
const minimumOutputTokens = 256

// SkippedModel records a model that was excluded from a capacity decision and why.
type SkippedModel struct {
	ModelID string `json:"model_id"`
	Reason  string `json:"reason"`
}

// CapacityDecision is the outcome of filtering a model chain down to those that
// can serve a request of the given input size and output budget.
type CapacityDecision struct {
	Models             []config.ModelConfig
	Skipped            []SkippedModel
	InputTokens        int
	RequestedMaxTokens int
	SelectedMaxTokens  int
	ContextWindow      int
	ContextMargin      int
}

// FilterByCapacity drops models whose context window cannot accommodate the
// request's input plus a minimum output floor, and clamps each surviving
// model's MaxTokens to what the request and the model can actually produce.
//
// Capacities are driven by per-model config fields (ContextWindow /
// ContextMargin / MaxOutputTokens); a model with ContextWindow ≤ 0 is treated
// as unconstrained and never filtered on capacity.
//
// A client that requests a small max_tokens (e.g. Claude Code's safety
// classifier asks for 64 tokens to render a yes/no verdict) does NOT cause a
// skip — the model still has room, it just needs to produce fewer tokens.
// A model is only ineligible when its own context window is exhausted below
// the output floor.
func FilterByCapacity(chain []config.ModelConfig, inputTokens int, requestedMaxTokens int) (CapacityDecision, error) {
	decision := CapacityDecision{
		InputTokens:        inputTokens,
		RequestedMaxTokens: requestedMaxTokens,
	}

	for _, model := range chain {
		// Capacity skip applies only when the model declares a context window
		// and its window cannot hold the input plus the output floor.
		if model.ContextWindow > 0 {
			remaining := model.ContextWindow - inputTokens - model.ContextMargin
			if remaining < minimumOutputTokens {
				decision.Skipped = append(decision.Skipped, SkippedModel{ModelID: model.ModelID, Reason: "context_window_exceeded"})
				continue
			}
		}

		// Even without a declared context window, clamp the effective
		// max_tokens against the client request and any model-level output cap.
		sentMax := clampOutputTokens(model, inputTokens, requestedMaxTokens)
		model.MaxTokens = sentMax
		if len(decision.Models) == 0 {
			decision.SelectedMaxTokens = sentMax
			decision.ContextWindow = model.ContextWindow
			decision.ContextMargin = model.ContextMargin
		}
		decision.Models = append(decision.Models, model)
	}

	if len(decision.Models) == 0 {
		return decision, fmt.Errorf("no eligible model for request capacity")
	}
	return decision, nil
}

// clampOutputTokens bounds the effective max_tokens for a model to the least of
// the client request, the model-level output cap, and the room left in the
// context window after accounting for input tokens.
func clampOutputTokens(model config.ModelConfig, inputTokens int, requestedMaxTokens int) int {
	if inputTokens < 0 {
		inputTokens = 0
	}
	limit := model.MaxTokens
	if requestedMaxTokens > 0 && (limit == 0 || requestedMaxTokens < limit) {
		limit = requestedMaxTokens
	}
	if model.MaxOutputTokens > 0 && (limit == 0 || model.MaxOutputTokens < limit) {
		limit = model.MaxOutputTokens
	}
	if model.ContextWindow <= 0 {
		return limit
	}
	remaining := model.ContextWindow - inputTokens - model.ContextMargin
	if limit == 0 || remaining < limit {
		if remaining < 0 {
			return 0
		}
		limit = remaining
	}
	return limit
}
