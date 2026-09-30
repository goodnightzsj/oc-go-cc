package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

type responseContractTransport func(*http.Request) (*http.Response, error)

func (f responseContractTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func responseContractProviders(body string) []core.Provider {
	cfg := config.NewAtomicConfig(&config.Config{
		OpenCodeGo:  config.OpenCodeGoConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat", ResponsesBaseURL: "https://synthetic.invalid/responses"},
		OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat", ResponsesBaseURL: "https://synthetic.invalid/responses"},
		AWSBedrock:  config.AWSBedrockConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/v1"},
		OpenRouter:  config.OpenRouterConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
		CommandCode: config.CommandCodeConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
	}, "")
	httpClient := &http.Client{Transport: responseContractTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	goProvider := NewOpenCodeGoProvider(cfg, nil)
	zen := NewOpenCodeZenProvider(cfg)
	bedrock := NewAWSBedrockProvider(cfg)
	openrouter := NewOpenRouterProvider(cfg, nil)
	commandcode := NewCommandCodeProvider(cfg, nil)
	for _, base := range []*baseProvider{&goProvider.baseProvider, &zen.baseProvider, &bedrock.baseProvider, &openrouter.baseProvider, &commandcode.baseProvider} {
		base.httpClient = httpClient
	}
	return []core.Provider{goProvider, zen, bedrock, openrouter, commandcode}
}

func TestResponsesTerminalStatusContract(t *testing.T) {
	for _, status := range []string{"completed", "incomplete", "failed", "cancelled"} {
		details, stopReason := "", "end_turn"
		if status == "incomplete" {
			details, stopReason = `,"incomplete_details":{"reason":"max_output_tokens"}`, "max_tokens"
		}
		body := fmt.Sprintf(`{"id":"synthetic","status":%q%s,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}],"usage":{"input_tokens":101,"output_tokens":7,"input_tokens_details":{"cached_tokens":90}}}`, status, details)
		for _, p := range responseContractProviders(body)[:3] {
			t.Run(p.Name()+"/"+status, func(t *testing.T) {
				result, err := p.Execute(context.Background(), &types.MessageRequest{MaxTokens: 16}, config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "responses"})
				if status == "failed" || status == "cancelled" {
					if err == nil {
						t.Fatalf("upstream %s became success: %s", status, result.Body)
					}
					var failure *core.ResponsesStatusError
					if !errors.As(err, &failure) || failure.Status != status || failure.Usage == nil || failure.Usage.InputTokens != 11 || failure.Usage.CacheReadInputTokens != 90 || failure.Usage.OutputTokens != 7 {
						t.Fatalf("failed status/usage lost: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var got types.MessageResponse
				if err := json.Unmarshal(result.Body, &got); err != nil {
					t.Fatal(err)
				}
				if got.StopReason != stopReason || got.Usage.InputTokens != 11 || got.Usage.CacheReadInputTokens != 90 || got.Usage.OutputTokens != 7 {
					t.Fatalf("completion/usage changed: %+v", got)
				}
			})
		}
	}
}

func TestChatResponseContractAcrossProviders(t *testing.T) {
	for _, usage := range []string{
		`{"prompt_tokens":101,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":90}}`,
		`{"prompt_tokens":101,"completion_tokens":7,"prompt_cache_hit_tokens":90,"prompt_cache_miss_tokens":11}`,
	} {
		body := `{"id":"synthetic","choices":[{"message":{"role":"assistant","content":"answer","reasoning_content":"thinking","tool_calls":[{"id":"call","type":"function","function":{"name":"lookup","arguments":""}}]},"finish_reason":"tool_calls"}],"usage":` + usage + `}`
		for _, p := range responseContractProviders(body) {
			t.Run(p.Name()+"/"+usage, func(t *testing.T) {
				result, err := p.Execute(context.Background(), &types.MessageRequest{MaxTokens: 16}, config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "openai"})
				if err != nil {
					t.Fatal(err)
				}
				var got types.MessageResponse
				if err := json.Unmarshal(result.Body, &got); err != nil {
					t.Fatal(err)
				}
				if len(got.Content) != 3 || got.Content[0].Type != "thinking" || got.Content[0].Thinking != "thinking" || got.Content[1].Type != "tool_use" || got.Content[1].ID != "call" || string(got.Content[1].Input) != "{}" || got.Content[2].Type != "text" || got.Content[2].Text != "answer" {
					t.Errorf("thinking/tool/text order or empty arguments changed: %s", result.Body)
				}
				if got.StopReason != "tool_use" || got.Usage.InputTokens != 11 || got.Usage.CacheReadInputTokens != 90 || got.Usage.CacheCreationInputTokens != 0 || got.Usage.OutputTokens != 7 {
					t.Errorf("stop/usage changed: %+v", got)
				}
			})
		}
	}
	for _, p := range responseContractProviders(`{"choices":[]}`) {
		t.Run(p.Name()+"/empty-choices", func(t *testing.T) {
			result, err := p.Execute(context.Background(), &types.MessageRequest{MaxTokens: 16}, config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "openai"})
			if err == nil {
				t.Fatalf("empty choices became success: %s", result.Body)
			}
		})
	}
}
