package transformer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestGeminiUsesConfiguredBudgetLimit(t *testing.T) {
	limit := 1024
	req := &types.MessageRequest{Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":8192}`)}
	model := config.ModelConfig{Provider: "opencode-zen", ModelID: "gemini-2.5-flash", Reasoning: &config.ReasoningConfig{MaxBudget: &limit}}
	got := mustGeminiRequest(t, req, model).GenerationConfig.ThinkingConfig
	if got == nil || got.ThinkingBudget == nil || *got.ThinkingBudget != limit {
		t.Fatalf("configured budget limit ignored: %+v", got)
	}
}

func TestGeminiThinkingDefaultDoesNotOverrideRequest(t *testing.T) {
	model := config.ModelConfig{Provider: "opencode-zen", ModelID: "gemini-2.5-flash", Thinking: json.RawMessage(`{"type":"enabled","budget_tokens":2048}`)}
	for _, explicit := range []bool{false, true} {
		req := &types.MessageRequest{}
		want := 2048
		if explicit {
			req.Thinking = json.RawMessage(`{"type":"disabled"}`)
			want = 0
		}
		got := mustGeminiRequest(t, req, model).GenerationConfig.ThinkingConfig
		if got == nil || got.ThinkingBudget == nil || *got.ThinkingBudget != want {
			t.Fatalf("explicit=%t thinking=%+v want=%d", explicit, got, want)
		}
	}
}

func TestImplicitReasoningDefaultUsesConfiguredLadder(t *testing.T) {
	model := config.ModelConfig{ModelID: "global:deepseek-v4.1-flash", Thinking: json.RawMessage(`{"type":"enabled"}`), Reasoning: &config.ReasoningConfig{Levels: []string{"low"}}}
	got, err := AnthropicToChatCompletion(&types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}, model)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReasoningEffort == nil || *got.ReasoningEffort != "low" {
		t.Fatalf("implicit default ignores operator ladder: %+v", got.ReasoningEffort)
	}
}

func TestCannotDisableAppliesToEveryThinkingSource(t *testing.T) {
	for _, source := range []string{"request switch", "request effort", "config switch", "config effort"} {
		t.Run(source, func(t *testing.T) {
			model := config.ModelConfig{ModelID: "global:deepseek-v4.1-flash", Reasoning: &config.ReasoningConfig{CannotDisable: config.BoolPtr(true)}}
			req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
			switch source {
			case "request switch":
				req.Thinking = json.RawMessage(`{"type":"disabled"}`)
				req.OutputConfig = json.RawMessage(`{"effort":"high"}`)
			case "request effort":
				req.OutputConfig = json.RawMessage(`{"effort":"none"}`)
			case "config switch":
				model.Thinking = json.RawMessage(`{"type":"disabled"}`)
			case "config effort":
				model.ReasoningEffort = "none"
			}
			got, err := AnthropicToChatCompletion(req, model)
			if err != nil {
				t.Fatal(err)
			}
			if isThinkingDisabled(got.Thinking) || got.ReasoningEffort == nil || *got.ReasoningEffort != "low" {
				t.Fatalf("cannot_disable ignored: thinking=%s effort=%v", got.Thinking, got.ReasoningEffort)
			}
		})
	}
}

func TestCannotDisableDoesNotBypassDeepSeekHistorySafety(t *testing.T) {
	model := config.ModelConfig{ModelID: "global:deepseek-v4.1-flash", Reasoning: &config.ReasoningConfig{CannotDisable: config.BoolPtr(true)}}
	req := &types.MessageRequest{Thinking: json.RawMessage(`{"type":"disabled"}`), Messages: []types.Message{{Role: "assistant", Content: json.RawMessage(`"previous answer"`)}}}
	if _, err := AnthropicToChatCompletion(req, model); err == nil || !strings.Contains(err.Error(), "assistant history") {
		t.Fatalf("conflicting history and capability must fail explicitly: %v", err)
	}
}

func TestChatBudgetLimitsApplyToRequestAndConfig(t *testing.T) {
	for _, source := range []string{"request", "config", "history"} {
		t.Run(source, func(t *testing.T) {
			limit := 1024
			model := config.ModelConfig{ModelID: "global:deepseek-v4.1-flash", Reasoning: &config.ReasoningConfig{MaxBudget: &limit}}
			req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
			budget := json.RawMessage(`{"type":"enabled","budget_tokens":8192}`)
			if source == "request" {
				req.Thinking = budget
			} else {
				model.Thinking = budget
				if source == "history" {
					req.Messages = append(req.Messages, types.Message{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking","thinking":"earlier"},{"type":"text","text":"answer"}]`)})
				}
			}
			got, err := AnthropicToChatCompletion(req, model)
			if err != nil || got == nil || parseBudgetTokens(got.Thinking) != limit {
				t.Fatalf("budget limit ignored: request=%+v err=%v", got, err)
			}
		})
	}
}
