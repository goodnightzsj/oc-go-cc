package core

import (
	"encoding/json"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

// Thinking.Type is a mode ("enabled"/"disabled"), never an effort level. It was
// stored straight into ReasoningEffort, putting a value the field's own contract
// forbids into every normalized request - and a consumer reading it as an effort
// would send "enabled" to a provider that only accepts levels.
func TestNormalizeRequestNeverStoresAModeAsEffort(t *testing.T) {
	for _, tc := range []struct {
		name       string
		thinking   string
		wantEffort string
		wantBudget int
	}{
		{"enabled with budget", `{"type":"enabled","budget_tokens":4096}`, "medium", 4096},
		{"enabled without budget", `{"type":"enabled"}`, "", 0},
		{"disabled", `{"type":"disabled"}`, "", 0},
		{"absent", ``, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.MessageRequest{Model: "m", MaxTokens: 1024}
			if tc.thinking != "" {
				req.Thinking = json.RawMessage(tc.thinking)
			}
			nr := NormalizeRequest(req)

			if nr.ReasoningEffort == "enabled" || nr.ReasoningEffort == "disabled" {
				t.Fatalf("ReasoningEffort = %q, which is a thinking mode, not an effort level", nr.ReasoningEffort)
			}
			if nr.ReasoningEffort != tc.wantEffort {
				t.Errorf("ReasoningEffort = %q, want %q", nr.ReasoningEffort, tc.wantEffort)
			}
			if nr.ThinkingBudget != tc.wantBudget {
				t.Errorf("ThinkingBudget = %d, want %d", nr.ThinkingBudget, tc.wantBudget)
			}
			// An effort that is not in the published set would be rejected or
			// silently ignored upstream, which is the failure this guards.
			if _, ok := NormalizeEffort(nr.ReasoningEffort); !ok {
				t.Errorf("ReasoningEffort %q is not a level this proxy accepts", nr.ReasoningEffort)
			}
		})
	}
}

// A zero budget means the client expressed no intent, which is not the same as
// asking for the weakest level.
func TestEffortFromBudgetReportsNothingForAZeroBudget(t *testing.T) {
	if got := EffortFromBudget(0); got != "" {
		t.Errorf("EffortFromBudget(0) = %q, want empty", got)
	}
	if got := EffortFromBudget(-10); got != "" {
		t.Errorf("EffortFromBudget(-10) = %q, want empty", got)
	}
}

// Every level either vendor documents is accepted, so a client cannot be told
// its normal settings are invalid. The value comes back unchanged: this
// function decides whether something is a level, not what a protocol can
// express. Mapping "none"/"minimal" onto Anthropic's ladder is AnthropicEffort's
// job, applied only at that boundary.
func TestNormalizeEffortAcceptsEveryDocumentedLevelUnchanged(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"none", "none"},
		{"minimal", "minimal"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"xhigh", "xhigh"},
		{"max", "max"},
	} {
		got, ok := NormalizeEffort(tc.in)
		if !ok {
			t.Errorf("%q was rejected", tc.in)
			continue
		}
		if got != tc.want {
			t.Errorf("%q -> %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A thinking mode is not a level. Sending "enabled" as an effort is the defect
// this function exists to prevent, so it must be rejected rather than passed on.
func TestNormalizeEffortRejectsModesAndUnknownValues(t *testing.T) {
	for _, value := range []string{"enabled", "disabled", "adaptive", "between_tools", "ultra", "HIGH", " high"} {
		if got, ok := NormalizeEffort(value); ok {
			t.Errorf("%q was accepted as a level (-> %q)", value, got)
		}
	}
}

// The raise belongs to the Anthropic boundary alone. Moving it back into
// NormalizeEffort would silently rewrite requests on the same-protocol path
// where nothing needed translating.
func TestAnthropicEffortRaisesOnlyAtThatBoundary(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"none", "low"},
		{"minimal", "low"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"xhigh", "xhigh"},
		{"max", "max"},
		{"", ""},
	} {
		if got := AnthropicEffort(tc.in); got != tc.want {
			t.Errorf("AnthropicEffort(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// And validation no longer changes the value it reports.
	for _, level := range []string{"none", "minimal"} {
		got, ok := NormalizeEffort(level)
		if !ok || got != level {
			t.Errorf("NormalizeEffort(%q) = (%q, %v); validation must not translate", level, got, ok)
		}
	}
}

// An operator declaration is additive over the built-in table: naming a ladder
// says nothing about the budget bounds, and dropping them would turn a
// correction into a regression.
func TestReasoningSupportForAppliesConfigOverrides(t *testing.T) {
	modelID := "global:deepseek-v4.1-flash"

	builtin := ReasoningSupport(modelID)
	if len(builtin.Levels) == 0 || !builtin.EffortDriven {
		t.Fatalf("builtin table lost its entry for %s: %+v", modelID, builtin)
	}

	// Only the ladder is overridden; the rest of the table survives.
	levels := []string{"low", "high"}
	budget := false
	override := ReasoningSupportFor(modelID, config.ModelConfig{
		ModelID:   modelID,
		Reasoning: &config.ReasoningConfig{Levels: levels, Budget: &budget},
	})
	if len(override.Levels) != 2 || override.Levels[1] != "high" {
		t.Errorf("levels = %v, want the operator's two", override.Levels)
	}
	if override.Budget {
		t.Error("the operator turned Budget off and it stayed on")
	}
	if !override.EffortDriven {
		t.Error("EffortDriven was not named by the operator, so it must survive")
	}

	// No override means the built-in table, unchanged.
	if got := ReasoningSupportFor(modelID, config.ModelConfig{ModelID: modelID}); len(got.Levels) != len(builtin.Levels) {
		t.Errorf("no override changed the ladder: %v", got.Levels)
	}

	// An override can also teach the table about a model it has never seen.
	unknown := ReasoningSupportFor("vendor/brand-new", config.ModelConfig{
		ModelID:   "vendor/brand-new",
		Reasoning: &config.ReasoningConfig{Levels: []string{"low", "medium"}},
	})
	if len(unknown.Levels) != 2 {
		t.Errorf("override on an unknown model = %v, want the declared ladder", unknown.Levels)
	}
	if got, ok := unknown.ClampLevel("max"); !ok || got != "medium" {
		t.Errorf("ClampLevel(max) = %q,%v; want the nearest declared rung medium", got, ok)
	}
}
