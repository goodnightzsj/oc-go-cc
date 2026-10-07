package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/pkg/types"
)

func observationProviders(t *testing.T, endpoint string) []core.Provider {
	t.Helper()
	cfg := config.NewAtomicConfig(&config.Config{
		OpenCodeGo:  config.OpenCodeGoConfig{APIKey: "synthetic", BaseURL: endpoint},
		OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic", BaseURL: endpoint},
		AWSBedrock:  config.AWSBedrockConfig{APIKey: "synthetic", BaseURL: endpoint},
		OpenRouter:  config.OpenRouterConfig{APIKey: "synthetic", BaseURL: endpoint},
		CommandCode: config.CommandCodeConfig{APIKey: "synthetic", BaseURL: endpoint},
		ClinePass:   config.ClinePassConfig{APIKey: "synthetic", BaseURL: endpoint},
	}, "")
	goProvider := NewOpenCodeGoProvider(cfg, nil)
	zen := NewOpenCodeZenProvider(cfg)
	bedrock := NewAWSBedrockProvider(cfg)
	openrouter := NewOpenRouterProvider(cfg, nil)
	commandcode := NewCommandCodeProvider(cfg, nil)
	cline := NewClinePassProvider(cfg, nil)
	for _, base := range []*baseProvider{&goProvider.baseProvider, &zen.baseProvider, &bedrock.baseProvider, &openrouter.baseProvider, &commandcode.baseProvider, &cline.baseProvider} {
		t.Cleanup(base.httpClient.CloseIdleConnections)
	}
	return []core.Provider{goProvider, zen, bedrock, openrouter, commandcode, cline}
}

func TestProviderUpstreamCallsCountHTTPAttempts(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		invalidURL bool
	}{
		{name: "success", status: http.StatusOK},
		{name: "http-rejection", status: http.StatusForbidden},
		{name: "redirect", status: http.StatusTemporaryRedirect},
		{name: "before-send", status: http.StatusOK, invalidURL: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var received atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				if tc.status == http.StatusTemporaryRedirect && r.URL.Path == "/start" {
					http.Redirect(w, r, "/final", tc.status)
					return
				}
				if tc.status == http.StatusForbidden {
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, `{"error":{"message":"synthetic rejection"}}`)
					return
				}
				var payload struct{ Stream bool }
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode upstream request: %v", err)
				}
				if payload.Stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"id\":\"synthetic\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
					return
				}
				_, _ = io.WriteString(w, `{"id":"synthetic","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}]}`)
			}))
			defer upstream.Close()
			endpoint := upstream.URL + "/start"
			if tc.invalidURL {
				endpoint = "://invalid"
			}
			for _, p := range observationProviders(t, endpoint) {
				for _, method := range []string{"Execute", "Stream"} {
					t.Run(p.Name()+"/"+method, func(t *testing.T) {
						m := metrics.New()
						ctx := m.WithUpstreamCounting(context.Background())
						model := config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "openai"}
						req := &types.MessageRequest{MaxTokens: 16, Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
						before := received.Load()
						var err error
						if method == "Execute" {
							_, err = p.Execute(ctx, req, model)
						} else {
							var body io.ReadCloser
							body, err = p.Stream(ctx, req, model)
							if err == nil {
								_, err = io.Copy(io.Discard, body)
								err = errors.Join(err, body.Close())
							}
						}
						wantCalls := int64(1)
						redirectBlocked := tc.status == http.StatusTemporaryRedirect && (p.Name() == "commandcode" || p.Name() == "cline-pass")
						if tc.invalidURL {
							wantCalls = 0
						} else if tc.status == http.StatusTemporaryRedirect && !redirectBlocked {
							wantCalls = 2
						}
						wantError := tc.invalidURL || tc.status == http.StatusForbidden || redirectBlocked
						if (err != nil) != wantError {
							t.Fatalf("error = %v, want error %v", err, wantError)
						}
						if tc.status == http.StatusForbidden || redirectBlocked {
							var apiErr *client.APIError
							if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status {
								t.Errorf("HTTP rejection contract changed: %v", err)
							}
						}
						got := m.GetSnapshot()
						if got.UpstreamCalls != wantCalls || received.Load()-before != wantCalls {
							t.Errorf("upstream_calls = %d, HTTP requests = %d, want %d", got.UpstreamCalls, received.Load()-before, wantCalls)
						}
						if got.RequestsReceived != 0 || got.RequestsSuccess != 0 || got.RequestsFailed != 0 || len(got.Latencies) != 0 || len(got.ModelCounts) != 0 {
							t.Errorf("provider attempt changed logical request metrics: %+v", got)
						}
						m.RecordSuccess(model.ModelID, time.Millisecond)
						if got := m.GetSnapshot().UpstreamCalls; got != wantCalls {
							t.Errorf("RecordSuccess changed upstream_calls to %d, want %d", got, wantCalls)
						}
					})
				}
			}
		})
	}
}

func TestProviderUpstreamCallsContextIsolation(t *testing.T) {
	var received atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	for _, p := range observationProviders(t, upstream.URL) {
		t.Run(p.Name(), func(t *testing.T) {
			first, second := metrics.New(), metrics.New()
			for _, ctx := range []context.Context{
				first.WithUpstreamCounting(context.Background()),
				second.WithUpstreamCounting(context.Background()),
				context.Background(),
			} {
				body, err := p.Stream(ctx, &types.MessageRequest{MaxTokens: 16}, config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "openai"})
				if err != nil {
					t.Fatal(err)
				}
				_, readErr := io.Copy(io.Discard, body)
				if err := errors.Join(readErr, body.Close()); err != nil {
					t.Fatal(err)
				}
			}
			if a, b := first.GetSnapshot().UpstreamCalls, second.GetSnapshot().UpstreamCalls; a != 1 || b != 1 {
				t.Errorf("request contexts leaked between metrics: first = %d, second = %d", a, b)
			}
		})
	}
	if got := received.Load(); got != 18 {
		t.Errorf("HTTP requests = %d, want 18 including untracked contexts", got)
	}
}
