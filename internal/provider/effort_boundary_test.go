package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/transformer"
)

func TestEffortWireBoundaries(t *testing.T) {
	for _, level := range []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", level, stream), func(t *testing.T) {
				req, err := transformer.ResponsesToMessageRequest([]byte(fmt.Sprintf(`{"model":"client-model","input":"hi","stream":%t,"max_output_tokens":64,"reasoning":{"effort":%q}}`, stream, level)))
				if err != nil {
					t.Fatal(err)
				}
				before, err := json.Marshal(req)
				if err != nil {
					t.Fatal(err)
				}
				model := config.ModelConfig{ModelID: "upstream-model"}
				chat, err := transformer.AnthropicToChatCompletion(req, model)
				if err != nil {
					t.Fatal(err)
				}
				chatEffort := ""
				if chat.ReasoningEffort != nil {
					chatEffort = *chat.ReasoningEffort
				}
				if chatEffort != level {
					t.Errorf("Chat effort=%q, want %q", chatEffort, level)
				}
				responses, err := transformer.AnthropicToResponses(req, model)
				if err != nil {
					t.Fatal(err)
				}
				responsesEffort := ""
				if responses.Reasoning != nil {
					responsesEffort = responses.Reasoning.Effort
				}
				if responsesEffort != level {
					t.Errorf("Responses effort=%q, want %q", responsesEffort, level)
				}
				wantNative := level
				if level == "none" || level == "minimal" {
					wantNative = "low"
				}
				for _, raw := range []bool{false, true} {
					ctx := context.Background()
					if raw {
						ctx = core.WithRequestMetadata(ctx, core.RequestMetadata{Body: before})
					}
					payload, err := anthropicPayload(ctx, req, model, stream)
					if err != nil {
						t.Fatal(err)
					}
					var out struct {
						Model        string `json:"model"`
						Stream       bool   `json:"stream"`
						OutputConfig *struct {
							Effort string `json:"effort"`
						} `json:"output_config"`
					}
					if err := json.Unmarshal(payload, &out); err != nil {
						t.Fatal(err)
					}
					got := ""
					if out.OutputConfig != nil {
						got = out.OutputConfig.Effort
					}
					if got != wantNative {
						t.Errorf("Anthropic raw=%t effort=%q, want %q", raw, got, wantNative)
					}
					if level == "" && out.OutputConfig != nil {
						t.Error("missing effort acquired output_config")
					}
					if out.Model != model.ModelID || out.Stream != stream {
						t.Errorf("model/stream changed: %s", payload)
					}
				}
				after, err := json.Marshal(req)
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != string(before) {
					t.Error("outbound mapping mutated the request")
				}
				if got := history.RequestedEffort(req); got != level {
					t.Errorf("history effort=%q, want %q", got, level)
				}
			})
		}
	}
}
