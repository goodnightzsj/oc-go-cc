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

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestBedrockProjectHeaderFollowsSelectedProtocol(t *testing.T) {
	for _, tc := range []struct{ model, override, format string }{
		{"anthropic.synthetic", "", "anthropic"},
		{"openai.synthetic", "", "openai"},
		{"openai.gpt-synthetic", "", "responses"},
		{"openai.gpt-synthetic", "anthropic", "anthropic"},
		{"anthropic.synthetic", "openai", "openai"},
		{"anthropic.synthetic", "responses", "responses"},
	} {
		for _, project := range []string{"", "synthetic-project"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/override=%s/project=%t/stream=%t", tc.model, tc.override, project != "", stream), func(t *testing.T) {
					cfg := config.NewAtomicConfig(&config.Config{AWSBedrock: config.AWSBedrockConfig{APIKey: "synthetic", ProjectID: project, BaseURL: "https://fixture.invalid/v1/chat/completions", AnthropicBaseURL: "https://fixture.invalid/v1/messages"}}, "")
					p := NewAWSBedrockProvider(cfg)
					calls := 0
					p.httpClient = &http.Client{Transport: responseContractTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						wantOpenAI, wantAnthropic := project, ""
						if tc.format == "anthropic" {
							wantOpenAI, wantAnthropic = "", project
						}
						if r.Header.Get("OpenAI-Project") != wantOpenAI || r.Header.Get("anthropic-workspace-id") != wantAnthropic {
							t.Errorf("wrong project header: OpenAI=%q Anthropic=%q; want %q/%q", r.Header.Get("OpenAI-Project"), r.Header.Get("anthropic-workspace-id"), wantOpenAI, wantAnthropic)
						}
						if r.Header.Get("Authorization") != "Bearer synthetic" || r.Header.Get("Content-Type") != "application/json" || r.Method != http.MethodPost {
							t.Error("request auth/content-type/method changed")
						}
						if (r.Header.Get("Accept") == "text/event-stream") != stream {
							t.Error("stream accept changed")
						}
						if (r.Header.Get("anthropic-version") == "2023-06-01") != (tc.format == "anthropic") {
							t.Error("anthropic header crossed protocols")
						}
						var payload map[string]any
						defer r.Body.Close()
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Error(err)
						}
						gotStream, _ := payload["stream"].(bool)
						if payload["model"] != tc.model || gotStream != stream {
							t.Errorf("model/stream changed: %v", payload)
						}
						if tc.format == "responses" {
							if payload["input"] == nil || !strings.HasSuffix(r.URL.Path, "/responses") {
								t.Error("Responses body/endpoint changed")
							}
						} else if payload["messages"] == nil {
							t.Error("Messages/Chat body changed")
						}
						body := `{"id":"synthetic","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`
						if tc.format == "openai" {
							body = `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
						}
						if tc.format == "responses" {
							body = `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`
						}
						if stream {
							body = "data: synthetic-source-bytes\n\n"
						}
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
					})}
					model := config.ModelConfig{Provider: p.Name(), ModelID: tc.model, WireFormat: tc.override}
					req := &types.MessageRequest{MaxTokens: 128, Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}}
					if stream {
						body, err := p.Stream(t.Context(), req, model)
						if err != nil {
							t.Fatal(err)
						}
						got, err := io.ReadAll(body)
						closeErr := body.Close()
						if err != nil || closeErr != nil || string(got) != "data: synthetic-source-bytes\n\n" {
							t.Fatalf("stream bytes changed: %s %v %v", got, err, closeErr)
						}
					} else if _, err := p.Execute(t.Context(), req, model); err != nil {
						t.Fatal(err)
					}
					if calls != 1 {
						t.Fatalf("calls=%d", calls)
					}
				})
			}
		}
	}
}

func TestBedrockWorkspacePreservesFailureIdentity(t *testing.T) {
	for _, format := range []string{"anthropic", "openai", "responses"} {
		for _, stream := range []bool{false, true} {
			for _, cancelled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/cancelled=%t", format, stream, cancelled), func(t *testing.T) {
					p := NewAWSBedrockProvider(config.NewAtomicConfig(&config.Config{AWSBedrock: config.AWSBedrockConfig{APIKey: "synthetic", ProjectID: "synthetic-project", BaseURL: "https://fixture.invalid/v1", AnthropicBaseURL: "https://fixture.invalid/v1/messages"}}, ""))
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					p.httpClient = &http.Client{Transport: responseContractTransport(func(r *http.Request) (*http.Response, error) {
						if cancelled {
							cancel()
							return nil, r.Context().Err()
						}
						return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"60"}}, Body: io.NopCloser(strings.NewReader("synthetic limit")), Request: r}, nil
					})}
					model := config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: format}
					req := &types.MessageRequest{MaxTokens: 128}
					var err error
					if stream {
						var body io.ReadCloser
						body, err = p.Stream(ctx, req, model)
						if body != nil {
							body.Close()
							t.Fatal("failure returned stream")
						}
					} else {
						value, executeErr := p.Execute(ctx, req, model)
						err = executeErr
						if value != nil {
							t.Fatal("failure returned response")
						}
					}
					if cancelled {
						if !errors.Is(err, context.Canceled) {
							t.Fatalf("cancellation identity lost: %v", err)
						}
						return
					}
					var apiErr *client.APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 || apiErr.Body != "synthetic limit" || apiErr.RetryAfter != "60" {
						t.Fatalf("source failure changed: %v", err)
					}
				})
			}
		}
	}
}
