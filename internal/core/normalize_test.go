package core

import (
	"encoding/json"
	"testing"

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

// The normalized request and the transformer must convert a budget the same
// way. If they drift, one reader is told "medium" while the upstream is told
// "high" for the same request, and nothing reports the disagreement.
func TestEffortFromBudgetMatchesTheTransformPath(t *testing.T) {
	// Mirrors transformer.budgetTokensToEffort. Duplicated rather than imported
	// because core must not depend on transformer; this test is what keeps the
	// two in step.
	transformPath := func(budget int) string {
		switch {
		case budget <= 2048:
			return "low"
		case budget <= 8192:
			return "medium"
		case budget <= 32768:
			return "high"
		default:
			return "max"
		}
	}
	for _, budget := range []int{1, 1024, 2048, 2049, 4096, 8192, 8193, 16384, 32768, 32769, 63999, 200000} {
		if got, want := EffortFromBudget(budget), transformPath(budget); got != want {
			t.Errorf("budget %d: core says %q, transformer says %q", budget, got, want)
		}
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
// its normal settings are invalid. The two rungs Anthropic lacks are raised to
// "low": the request asked for as little reasoning as possible, and that is the
// least this ladder can express.
func TestNormalizeEffortAcceptsEveryDocumentedLevelAndRaisesTheTwoLowest(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"none", "low"},
		{"minimal", "low"},
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
