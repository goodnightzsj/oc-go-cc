package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/router"
)

func TestMalformedSourceJSONContinuesFallback(t *testing.T) {
	for _, tc := range []struct {
		provider, format string
	}{
		{"workbuddy", "openai"}, {"openrouter", "openai"},
		{"opencode-go", "openai"}, {"opencode-zen", "openai"}, {"aws-bedrock", "openai"},
		{"opencode-go", "responses"}, {"opencode-zen", "responses"},
		{"aws-bedrock", "responses"}, {"commandcode", "responses"},
		{"opencode-zen", "gemini"}, {"opencode-go", "anthropic"},
	} {
		for _, native := range []bool{false, true} {
			if native && tc.format != "responses" {
				continue
			}
			name := tc.provider + "/" + tc.format
			if native {
				name += "/native"
			}
			t.Run(name, func(t *testing.T) {
				var requests atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", "application/json")
					payload := `{"id":"synthetic","choices":[`
					if requests.Add(1) > 1 {
						switch tc.format {
						case "responses":
							payload = `{"id":"second","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second succeeded"}]}]}`
						case "gemini":
							payload = `{"candidates":[{"content":{"role":"model","parts":[{"text":"second succeeded"}]},"finishReason":"STOP"}]}`
						case "anthropic":
							payload = `{"id":"second","type":"message","role":"assistant","model":"synthetic-next","content":[{"type":"text","text":"second succeeded"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`
						default:
							payload = `{"id":"second","object":"chat.completion","model":"synthetic-next","choices":[{"message":{"role":"assistant","content":"second succeeded"},"finish_reason":"stop"}]}`
						}
					}
					_, _ = io.WriteString(w, payload)
				}))
				defer upstream.Close()
				p, pool := selectedTestProvider(tc.provider, upstream.URL)
				_, _, req := cancellationTestProvider("")
				req.MaxTokens = 16
				ctx := t.Context()
				if native {
					ctx = core.WithRequestMetadata(ctx, core.RequestMetadata{ResponsesBody: []byte(`{"model":"synthetic","input":"hello"}`)})
				}
				model := config.ModelConfig{Provider: tc.provider, ModelID: "synthetic", WireFormat: tc.format}
				next := model
				next.ModelID = "synthetic-next"
				h := router.NewFallbackHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), 3, time.Minute)
				calls := 0
				_, body, err := h.ExecuteWithFallback(ctx, []config.ModelConfig{model, next}, func(ctx context.Context, candidate config.ModelConfig, _ int) ([]byte, error) {
					calls++
					result, failure := p.Execute(ctx, req, candidate)
					if calls == 1 {
						var source *core.ResponseError
						var syntax *json.SyntaxError
						if !errors.As(failure, &source) || !errors.As(failure, &syntax) || source.ReportedUsage() != nil {
							t.Errorf("source identity/unknown usage lost: %v", failure)
						}
						if pool.states["synthetic-a"].consecutive != 1 {
							t.Error("source failure must settle key health exactly once")
						}
					}
					if failure != nil {
						return nil, failure
					}
					return result.Body, nil
				})
				if err != nil || calls != 2 || requests.Load() != 2 || !strings.Contains(string(body), "second succeeded") {
					t.Fatalf("healthy fallback not served: calls=%d requests=%d body=%s err=%v", calls, requests.Load(), body, err)
				}
			})
		}
	}
}

func TestMalformedChatArgumentsContinueFallback(t *testing.T) {
	for _, name := range []string{"workbuddy", "openrouter", "opencode-go", "opencode-zen", "aws-bedrock", "commandcode"} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if calls.Add(1) == 1 {
					_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"lookup","arguments":"{\"q\":"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`)
					return
				}
				_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"second succeeded"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
			}))
			defer upstream.Close()
			p, pool := selectedTestProvider(name, upstream.URL)
			_, _, req := cancellationTestProvider("")
			req.MaxTokens = 16
			model := config.ModelConfig{Provider: name, ModelID: "synthetic", WireFormat: "openai"}
			next := model
			next.ModelID = "synthetic-next"
			h := router.NewFallbackHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), 3, time.Minute)
			attempts := 0
			_, body, err := h.ExecuteWithFallback(t.Context(), []config.ModelConfig{model, next}, func(ctx context.Context, candidate config.ModelConfig, _ int) ([]byte, error) {
				attempts++
				result, failure := p.Execute(ctx, req, candidate)
				if attempts == 1 {
					var source *core.ResponseError
					if !errors.As(failure, &source) || source.ReportedUsage() == nil || source.ReportedUsage().OutputTokens != 7 || pool.stateFor("synthetic-a").consecutive != 1 {
						t.Errorf("source error/usage/key outcome lost: %v", failure)
					}
				}
				if failure != nil {
					return nil, failure
				}
				return result.Body, nil
			})
			if err != nil || attempts != 2 || calls.Load() != 2 || !strings.Contains(string(body), "second succeeded") {
				t.Fatalf("fallback lost: attempts=%d requests=%d body=%s err=%v", attempts, calls.Load(), body, err)
			}
		})
	}
}
