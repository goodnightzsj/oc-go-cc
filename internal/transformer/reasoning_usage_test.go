package transformer

import (
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

// WorkBuddy answers with completion_tokens_details.reasoning_tokens; the
// DeepSeek-family gateways publish completion_thinking_tokens alongside it.
// Both name the same figure, and a count must not be lost to whichever spelling
// a particular upstream chose.
func TestReasoningTokensFromEitherSpelling(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage *types.UsageInfo
		want  int
	}{
		{"standard details", &types.UsageInfo{CompletionTokens: 36,
			CompletionTokensDetails: &types.CompletionTokensDetails{ReasoningTokens: 26}}, 26},
		{"deepseek spelling", &types.UsageInfo{CompletionTokens: 36, CompletionThinkingTokens: 26}, 26},
		{"both agree", &types.UsageInfo{CompletionTokens: 36, CompletionThinkingTokens: 26,
			CompletionTokensDetails: &types.CompletionTokensDetails{ReasoningTokens: 26}}, 26},
		{"details wins when both differ", &types.UsageInfo{CompletionTokens: 36, CompletionThinkingTokens: 5,
			CompletionTokensDetails: &types.CompletionTokensDetails{ReasoningTokens: 26}}, 26},
		{"neither", &types.UsageInfo{CompletionTokens: 36}, 0},
		{"nil usage", nil, 0},
	} {
		got := usageInfoToAnthropic(tc.usage)
		if got.ReasoningTokens != tc.want {
			t.Errorf("%s: ReasoningTokens = %d, want %d", tc.name, got.ReasoningTokens, tc.want)
		}
	}
}

// The split is part of the completion, not additional to it. Adding it would
// double-count every reasoning token in the billing path.
func TestReasoningTokensDoNotInflateOutput(t *testing.T) {
	got := usageInfoToAnthropic(&types.UsageInfo{
		PromptTokens: 56, CompletionTokens: 36,
		CompletionTokensDetails: &types.CompletionTokensDetails{ReasoningTokens: 26},
	})
	if got.OutputTokens != 36 {
		t.Errorf("OutputTokens = %d, want 36 (reasoning is inside it)", got.OutputTokens)
	}
}
