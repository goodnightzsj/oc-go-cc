package router

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
)

// The primary stays scoped to the active site - sending traffic elsewhere by
// default is a different decision than the one the operator made. Fallbacks are
// not scoped: a plan for "this platform cannot answer" that is limited to this
// platform cannot do its job.
func TestSplitKeepsSamePlatformFallbacksAhead(t *testing.T) {
	primary := config.ModelConfig{Provider: "cline-pass", ModelID: "primary"}
	fallbacks := []config.ModelConfig{
		{Provider: "commandcode", ModelID: "cross-one"},
		{Provider: "cline-pass", ModelID: "same-one"},
		{Provider: "commandcode", ModelID: "cross-two"},
		{Provider: "cline-pass", ModelID: "same-two"},
	}

	gotPrimary, ordered := SplitPrimaryAndFallbacks("cline-pass", primary, fallbacks)
	if gotPrimary.ModelID != "primary" {
		t.Errorf("primary changed: %q", gotPrimary.ModelID)
	}
	want := []string{"same-one", "same-two", "cross-one", "cross-two"}
	if len(ordered) != len(want) {
		t.Fatalf("chain = %d entries, want %d: %+v", len(ordered), len(want), ordered)
	}
	for i, w := range want {
		if ordered[i].ModelID != w {
			t.Errorf("position %d = %q, want %q (same-platform quota is spent first)", i, ordered[i].ModelID, w)
		}
	}
}

// An unscoped config keeps the order the operator wrote.
func TestSplitIsANoOpWithoutAnActiveSite(t *testing.T) {
	primary := config.ModelConfig{Provider: "a", ModelID: "p"}
	fallbacks := []config.ModelConfig{{Provider: "b", ModelID: "x"}, {Provider: "a", ModelID: "y"}}
	_, ordered := SplitPrimaryAndFallbacks("", primary, fallbacks)
	if ordered[0].ModelID != "x" || ordered[1].ModelID != "y" {
		t.Errorf("order changed with no active site: %+v", ordered)
	}
}

// A duplicate of the primary is not a fallback.
func TestSplitDropsThePrimaryFromItsOwnChain(t *testing.T) {
	primary := config.ModelConfig{Provider: "a", ModelID: "p"}
	fallbacks := []config.ModelConfig{primary, {Provider: "a", ModelID: "other"}}
	_, ordered := SplitPrimaryAndFallbacks("a", primary, fallbacks)
	if len(ordered) != 1 || ordered[0].ModelID != "other" {
		t.Errorf("chain = %+v, want only the distinct model", ordered)
	}
}
