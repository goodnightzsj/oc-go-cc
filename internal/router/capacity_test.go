package router

import (
	"testing"

	"oc-go-cc/internal/config"
)

func TestFilterByCapacity_SmallMaxTokens_NotSkipped(t *testing.T) {
	// Claude Code's auto-mode safety classifier sends a tiny non-streaming
	// request (max_tokens=64) to render a yes/no verdict. The model still has
	// ample context room; only its requested output budget is small. This must
	// NOT be treated as capacity-ineligible — otherwise auto mode can never
	// determine safety ("model temporarily unavailable").
	chain := []config.ModelConfig{
		{ModelID: "kimi-k2.6", Provider: "opencode-go", ContextWindow: 128000, ContextMargin: 4096},
	}

	decision, err := FilterByCapacity(chain, 1000, 64)
	if err != nil {
		t.Fatalf("small max_tokens request must not be rejected: %v", err)
	}
	if len(decision.Models) != 1 {
		t.Fatalf("got %d models, want 1 (small max_tokens must not cause a skip)", len(decision.Models))
	}
	// The clamp still applies to the sent max_tokens.
	if decision.Models[0].MaxTokens != 64 {
		t.Errorf("clamped MaxTokens = %d, want 64", decision.Models[0].MaxTokens)
	}
}

func TestFilterByCapacity_ContextExhausted_Skipped(t *testing.T) {
	// A request whose input leaves less than the output floor in the model's
	// context window IS ineligible — the model physically cannot respond.
	chain := []config.ModelConfig{
		{ModelID: "kimi-k2.6", Provider: "opencode-go", ContextWindow: 128000, ContextMargin: 4096},
	}

	decision, err := FilterByCapacity(chain, 130000, 4096)
	if err == nil {
		t.Fatalf("expected error when context window is exhausted, got none (models=%d)", len(decision.Models))
	}
	if len(decision.Skipped) != 1 || decision.Skipped[0].Reason != "context_window_exceeded" {
		t.Errorf("expected context_window_exceeded skip, got %+v", decision.Skipped)
	}
}

func TestFilterByCapacity_UnconstrainedModel_Passthrough(t *testing.T) {
	// A model that declares no capacity metadata (ContextWindow/MaxTokens<=0)
	// is passed through unchanged — this is the pre-existing behavior.
	chain := []config.ModelConfig{
		{ModelID: "kimi-k2.6", Provider: "opencode-go", MaxTokens: 4096},
	}

	decision, err := FilterByCapacity(chain, 90000, 64)
	if err != nil {
		t.Fatalf("unconstrained model must not be rejected: %v", err)
	}
	if len(decision.Models) != 1 {
		t.Fatalf("got %d models, want 1", len(decision.Models))
	}
	if decision.Models[0].MaxTokens != 64 {
		t.Errorf("clamped MaxTokens = %d, want 64 (requested max applies)", decision.Models[0].MaxTokens)
	}
}

func TestClampOutputTokens_ClientRequestWins(t *testing.T) {
	model := config.ModelConfig{ModelID: "m", ContextWindow: 128000, ContextMargin: 4096, MaxTokens: 8192}
	// Client asks for 256; model cap has no room to raise it.
	if got := clampOutputTokens(model, 1000, 256); got != 256 {
		t.Errorf("got %d, want 256", got)
	}
}

func TestClampOutputTokens_ModelOutputCapClamps(t *testing.T) {
	model := config.ModelConfig{ModelID: "m", ContextWindow: 0, ContextMargin: 0, MaxOutputTokens: 512}
	// Model-level output cap is tighter than the client's request.
	if got := clampOutputTokens(model, 1000, 1024); got != 512 {
		t.Errorf("got %d, want 512", got)
	}
	// Model-level cap is looser than the client's request — client wins.
	if got := clampOutputTokens(model, 1000, 256); got != 256 {
		t.Errorf("got %d, want 256", got)
	}
}
