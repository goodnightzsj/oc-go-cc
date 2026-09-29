package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestOpenRouterContract(t *testing.T) {
	for _, suffix := range []string{"", "/api/v1/", "/api/v1/chat/completions"} {
		t.Run(suffix, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				wantPath := "/api/v1/chat/completions"
				if suffix == "" {
					wantPath = "/"
				}
				wantKey := "Bearer synthetic-a"
				if calls%2 == 0 {
					wantKey = "Bearer synthetic-b"
				}
				if r.URL.Path != wantPath || r.Header.Get("Authorization") != wantKey {
					t.Error("endpoint or dedicated key rotation changed")
				}
				for key, want := range map[string]string{
					"HTTP-Referer": "https://github.com/routatic/proxy", "X-OpenRouter-Title": "routatic-proxy",
					"X-OpenRouter-Categories": "cli-agent", "User-Agent": "routatic-proxy",
					"x-api-key": "", "x-opencode-session": "", "anthropic-beta": "",
				} {
					if r.Header.Get(key) != want {
						t.Errorf("wrong %s header", key)
					}
				}
				var req types.ChatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.Model != "anthropic/claude-test" || len(req.Messages) != 1 || req.Messages[0].ContentText() != "hello" {
					t.Errorf("converted request = %+v", req)
				}
				if req.Stream != nil && *req.Stream {
					if r.Header.Get("Accept") != "text/event-stream" {
						t.Error("missing streaming Accept")
					}
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
					return
				}
				_, _ = io.WriteString(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":101,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":90}}}`)
			}))
			defer upstream.Close()
			cfg := &config.Config{APIKey: "synthetic-global", OpenRouter: config.OpenRouterConfig{
				BaseURL: upstream.URL + suffix, APIKey: "synthetic-a", APIKeys: []string{"synthetic-b"}, ManagementAPIKey: "synthetic-management",
			}}
			p := NewOpenRouterProvider(config.NewAtomicConfig(cfg, ""), nil)
			model := config.ModelConfig{Provider: p.Name(), ModelID: "anthropic/claude-test"}
			req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
			ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{SessionID: "synthetic-session", AnthropicBeta: "synthetic-beta"})
			result, err := p.Execute(ctx, req, model)
			if err != nil {
				t.Fatal(err)
			}
			var response types.MessageResponse
			if err := json.Unmarshal(result.Body, &response); err != nil {
				t.Fatal(err)
			}
			if response.Usage.InputTokens != 11 || response.Usage.CacheReadInputTokens != 90 || response.Usage.OutputTokens != 7 || response.StopReason != "end_turn" {
				t.Fatalf("conversion changed: %+v", response)
			}
			body, err := p.Stream(ctx, req, model)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(body)
			_ = body.Close()
			if err != nil || string(raw) != "data: [DONE]\n\n" {
				t.Fatalf("raw stream changed: %q %v", raw, err)
			}
			if calls != 2 || req.Stream != nil {
				t.Fatal("request count or caller-owned stream flag changed")
			}
		})
	}
}

func TestOpenRouterFailures(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "buffered", true: "stream"}[stream], func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer synthetic-global" {
					t.Error("existing global key fallback changed")
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, "synthetic refusal")
			}))
			defer upstream.Close()
			cfg := &config.Config{APIKey: "synthetic-global", OpenRouter: config.OpenRouterConfig{BaseURL: upstream.URL}}
			p := NewOpenRouterProvider(config.NewAtomicConfig(cfg, ""), nil)
			model := config.ModelConfig{Provider: p.Name(), ModelID: "synthetic"}
			call := func(ctx context.Context) error {
				if stream {
					body, err := p.Stream(ctx, &types.MessageRequest{}, model)
					if body != nil {
						_ = body.Close()
					}
					return err
				}
				_, err := p.Execute(ctx, &types.MessageRequest{}, model)
				return err
			}
			err := call(context.Background())
			var apiErr *client.APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 || apiErr.Body != "synthetic refusal" {
				t.Fatalf("upstream error lost: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := call(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			model.WireFormat = "anthropic"
			if err := call(context.Background()); err == nil || !strings.Contains(err.Error(), "wire format") {
				t.Fatalf("unsupported format accepted: %v", err)
			}
			if calls != 1 {
				t.Fatalf("invalid/canceled requests sent upstream: %d", calls)
			}
		})
	}
}
