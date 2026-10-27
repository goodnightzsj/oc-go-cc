package transformer

import (
	"context"
	"fmt"
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestStreamReasoningUsesLatestObservedSplit(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		want         int
	}{
		{"missing", "", 26},
		{"zero", `,"completion_tokens_details":{"reasoning_tokens":0}`, 26},
		{"updated details", `,"completion_tokens_details":{"reasoning_tokens":30}`, 30},
		{"updated alias", `,"completion_thinking_tokens":28`, 28},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := sseLines(
				`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":30,"completion_tokens_details":{"reasoning_tokens":26}}}`,
				fmt.Sprintf(`{"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":101,"completion_tokens":36,"prompt_tokens_details":{"cached_tokens":90}%s}}`, tc.fields),
				`{"choices":[],"usage":null}`, "[DONE]",
			)
			w := newMockResponseWriter()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := NewStreamHandler().ProxyStream(w, body, "synthetic", ctx, 0, cancel); err != nil {
				t.Fatal(err)
			}
			events := parseSSEEvents(t, w.buf.String())
			var usage *types.Usage
			for _, event := range events {
				if event.Type == "message_delta" {
					usage = event.Usage
				}
			}
			if usage == nil || usage.ReasoningTokens != tc.want || usage.InputTokens != 11 || usage.CacheReadInputTokens != 90 || usage.OutputTokens != 36 {
				t.Fatalf("terminal usage = %+v, want latest totals and reasoning %d", usage, tc.want)
			}
		})
	}
}

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
		got := ChatUsageToAnthropic(tc.usage)
		if tc.usage == nil {
			if got != nil {
				t.Error("absent usage became observed zero")
			}
			continue
		}
		if got.ReasoningTokens != tc.want {
			t.Errorf("%s: ReasoningTokens = %d, want %d", tc.name, got.ReasoningTokens, tc.want)
		}
	}
}

// The split is part of the completion, not additional to it. Adding it would
// double-count every reasoning token in the billing path.
func TestReasoningTokensDoNotInflateOutput(t *testing.T) {
	got := ChatUsageToAnthropic(&types.UsageInfo{
		PromptTokens: 56, CompletionTokens: 36,
		CompletionTokensDetails: &types.CompletionTokensDetails{ReasoningTokens: 26},
	})
	if got.OutputTokens != 36 {
		t.Errorf("OutputTokens = %d, want 36 (reasoning is inside it)", got.OutputTokens)
	}
}
