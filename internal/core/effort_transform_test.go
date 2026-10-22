package core_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

func TestBudgetEffortAcrossRealBoundaries(t *testing.T) {
	for _, tc := range []struct {
		budget int
		want   string
	}{
		{-1, ""}, {0, ""}, {1, "low"}, {2047, "low"}, {2048, "low"},
		{2049, "medium"}, {8191, "medium"}, {8192, "medium"}, {8193, "high"},
		{32767, "high"}, {32768, "high"}, {32769, "max"}, {63999, "max"},
	} {
		t.Run(fmt.Sprint(tc.budget), func(t *testing.T) {
			req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}, Thinking: json.RawMessage(fmt.Sprintf(`{"type":"enabled","budget_tokens":%d}`, tc.budget))}
			if got := core.NormalizeRequest(req).ReasoningEffort; got != tc.want {
				t.Fatalf("normalize=%q want=%q", got, tc.want)
			}
			if got := history.RequestedEffort(req); got != tc.want {
				t.Fatalf("history=%q want=%q", got, tc.want)
			}
			wire, err := transformer.AnthropicToChatCompletion(req, config.ModelConfig{ModelID: "o3-test"})
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if wire.ReasoningEffort != nil {
				got = *wire.ReasoningEffort
			}
			if got != tc.want {
				t.Fatalf("wire=%q want=%q", got, tc.want)
			}
		})
	}
}
