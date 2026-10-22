package router

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestFilterByCapacitySkipsPrimaryAndUsesEligibleFallback(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "glm-5.1", MaxTokens: 8192},
		{Provider: "opencode-go", ModelID: "deepseek-v4-pro", MaxTokens: 8192},
	}

	decision, err := FilterByCapacity(chain, 250000, 8192, false, false)
	if err != nil {
		t.Fatalf("FilterByCapacity() error = %v", err)
	}
	if got, want := decision.Models[0].ModelID, "deepseek-v4-pro"; got != want {
		t.Fatalf("selected model = %s, want %s", got, want)
	}
	if len(decision.Skipped) != 1 || decision.Skipped[0].Reason != "context_window_exceeded" {
		t.Fatalf("skipped = %+v, want context skip", decision.Skipped)
	}
}

func TestFilterByCapacityRejectsVisionFallbackToTextModel(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "deepseek-v4-pro", MaxTokens: 8192},
	}

	decision, err := FilterByCapacity(chain, 1000, 8192, true, false)
	if err == nil {
		t.Fatal("FilterByCapacity() error = nil, want error")
	}
	if len(decision.Models) != 0 {
		t.Fatalf("eligible models = %+v, want none", decision.Models)
	}
	if len(decision.Skipped) != 1 || decision.Skipped[0].Reason != "vision_not_supported" {
		t.Fatalf("skipped = %+v, want vision skip", decision.Skipped)
	}
}

func TestFilterByCapacityClampsMaxTokens(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6", MaxTokens: 16384},
	}

	decision, err := FilterByCapacity(chain, 240000, 16384, true, false)
	if err != nil {
		t.Fatalf("FilterByCapacity() error = %v", err)
	}
	if got, want := decision.Models[0].MaxTokens, 256000-240000-config.DefaultContextMargin; got != want {
		t.Fatalf("max_tokens = %d, want %d", got, want)
	}
}

// TestFilterByCapacityHonorsSmallMaxTokens guards the auto-mode classifier
// regression: the harness's safety classifier sends a tiny non-streaming
// request (max_tokens=64) to render a yes/no verdict. The capacity filter
// must keep the model eligible — the model has ample context room, it just
// needs to produce few output tokens — rather than rejecting it with "no
// eligible model for request capacity", which the harness reports as
// "model temporarily unavailable, auto mode cannot determine safety".
func TestFilterByCapacityHonorsSmallMaxTokens(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "glm-5.2", MaxTokens: 8192},
	}

	decision, err := FilterByCapacity(chain, 500, 64, false, false)
	if err != nil {
		t.Fatalf("FilterByCapacity() error = %v, want nil (small max_tokens must not skip)", err)
	}
	if len(decision.Models) != 1 {
		t.Fatalf("eligible models = %+v, want exactly 1", decision.Models)
	}
	if got, want := decision.Models[0].MaxTokens, 64; got != want {
		t.Fatalf("max_tokens = %d, want %d (client's small request honored)", got, want)
	}
	if len(decision.Skipped) != 0 {
		t.Fatalf("skipped = %+v, want none for small max_tokens with room to spare", decision.Skipped)
	}
}

// A model no source describes must not be refused an image request.
//
// This is the defect that made a vision-capable model answer 500: the catalog
// had not indexed it, Vision read false, and the chain's only model was dropped
// as "vision_not_supported" before the platform was ever asked. An unstated
// capability is not a refusal.
func TestFilterByCapacityKeepsModelWhoseVisionIsUnknown(t *testing.T) {
	chain := []config.ModelConfig{
		{Provider: "cline-pass", ModelID: "some-model-nobody-catalogued"},
	}
	decision, err := FilterByCapacity(chain, 1000, 4096, true, false)
	if err != nil {
		t.Fatalf("an unknown capability must not fail the request: %v", err)
	}
	if len(decision.Models) != 1 {
		t.Fatalf("kept %d models, want the one unknown model", len(decision.Models))
	}
	if len(decision.Skipped) != 0 {
		t.Errorf("skipped = %+v, want none", decision.Skipped)
	}
}

// A model known to lack vision is still skipped - that is what the filter is
// for, and it lets the chain fall through to a model that can serve the image.
func TestFilterByCapacityStillSkipsAKnownTextOnlyModel(t *testing.T) {
	textOnly := false
	chain := []config.ModelConfig{
		{Provider: "cline-pass", ModelID: "known-text-only", Vision: &textOnly},
	}
	decision, err := FilterByCapacity(chain, 1000, 4096, true, false)
	if err == nil {
		t.Fatal("a known text-only model must still be skipped")
	}
	if len(decision.Skipped) != 1 || decision.Skipped[0].Reason != "vision_not_supported" {
		t.Errorf("skipped = %+v, want vision_not_supported", decision.Skipped)
	}
}
