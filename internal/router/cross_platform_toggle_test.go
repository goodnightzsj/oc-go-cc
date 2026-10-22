package router

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
)

// The count is what makes the feature measurable before it is enabled: an
// operator watches how often a cross-platform fallback would fire without
// spending another account's quota to find out.
func TestClassifyChainCountsCrossPlatformCandidates(t *testing.T) {
	primary := config.ModelConfig{Provider: "cline-pass", ModelID: "primary"}
	fallbacks := []config.ModelConfig{
		{Provider: "commandcode", ModelID: "c1"},
		{Provider: "cline-pass", ModelID: "s1"},
		{Provider: "openrouter", ModelID: "c2"},
	}

	cross, ordered := ClassifyChain("cline-pass", primary, fallbacks)
	if cross != 2 {
		t.Errorf("cross = %d, want 2", cross)
	}
	if len(ordered) != 3 {
		t.Fatalf("ordered = %d entries; the count must not drop anything", len(ordered))
	}
	if ordered[0].ModelID != "s1" {
		t.Errorf("same-platform entry must lead, got %q", ordered[0].ModelID)
	}
}

// With the switch off, the chain is truncated to the active site. The suffix
// ClassifyChain put at the end is exactly what has to go, so a plain filter is
// enough and no execution path has to learn about the switch.
func TestTruncatingAfterClassifyKeepsOnlyTheActiveSite(t *testing.T) {
	primary := config.ModelConfig{Provider: "cline-pass", ModelID: "primary"}
	fallbacks := []config.ModelConfig{
		{Provider: "commandcode", ModelID: "c1"},
		{Provider: "cline-pass", ModelID: "s1"},
	}
	_, ordered := ClassifyChain("cline-pass", primary, fallbacks)

	kept := RestrictToActiveSite("cline-pass", ordered)
	if len(kept) != 1 || kept[0].ModelID != "s1" {
		t.Errorf("kept = %+v, want only the same-platform entry", kept)
	}
}

// No active site means no boundary, so nothing is ever cross-platform.
func TestClassifyChainWithoutAnActiveSite(t *testing.T) {
	cross, ordered := ClassifyChain("", config.ModelConfig{Provider: "a", ModelID: "p"},
		[]config.ModelConfig{{Provider: "b", ModelID: "x"}})
	if cross != 0 {
		t.Errorf("cross = %d with no active site, want 0", cross)
	}
	if len(ordered) != 1 {
		t.Errorf("ordered changed with no active site: %+v", ordered)
	}
}
