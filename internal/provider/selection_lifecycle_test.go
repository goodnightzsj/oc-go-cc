package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

func TestZenAnthropicDoesNotRetryRejectedKey(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "execute"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") == "Bearer synthetic-rejected" {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"error":"rejected"}`)
					return
				}
				_, _ = io.WriteString(w, `{"type":"message","content":[{"type":"text","text":"ok"}]}`)
			}))
			defer upstream.Close()
			cfg := &config.Config{OpenCodeZen: config.OpenCodeZenConfig{AnthropicBaseURL: upstream.URL, APIKeys: []string{"synthetic-rejected", "synthetic-healthy"}}}
			p := NewOpenCodeZenProvider(config.NewAtomicConfig(cfg, ""))
			model := config.ModelConfig{Provider: p.Name(), ModelID: "claude-test"}
			_, _, req := cancellationTestProvider("")
			req.MaxTokens = 16
			attempt := func() error {
				if stream {
					body, err := p.Stream(context.Background(), req, model)
					if body != nil {
						_, _ = io.Copy(io.Discard, body)
						_ = body.Close()
					}
					return err
				}
				_, err := p.Execute(context.Background(), req, model)
				return err
			}
			if err := attempt(); err == nil {
				t.Fatal("initial rejected key unexpectedly succeeded")
			}
			for i := 2; i <= 4; i++ {
				if err := attempt(); err != nil {
					t.Fatalf("attempt %d reused rejected key instead of staying with healthy key: %v", i, err)
				}
			}
		})
	}
}

func TestProviderSelectionLifecycle(t *testing.T) {
	for _, name := range []string{"opencode-go", "opencode-zen", "aws-bedrock", "openrouter", "commandcode", "cline-pass"} {
		formats := []core.WireFormat{core.WireFormatOpenAIChat}
		if name != "openrouter" && name != "cline-pass" {
			formats = append(formats, core.WireFormatAnthropic)
		}
		if name == "opencode-go" || name == "opencode-zen" || name == "aws-bedrock" {
			formats = append(formats, core.WireFormatOpenAIResponses)
		}
		if name == "opencode-zen" {
			formats = append(formats, core.WireFormatGemini)
		}
		for _, format := range formats {
			for _, stream := range []bool{false, true} {
				for _, failure := range []string{"invalid_url", "auth", "rate_limit", "canceled"} {
					t.Run(name+"/"+format.String()+"/"+failure+fmtStream(stream), func(t *testing.T) {
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Retry-After", "120")
							if failure == "auth" {
								w.WriteHeader(401)
							} else {
								w.WriteHeader(429)
							}
						}))
						defer upstream.Close()
						endpoint := upstream.URL
						if failure == "invalid_url" {
							endpoint = "://bad"
						}
						keys := []string{"synthetic-a", "synthetic-b"}
						cfg := &config.Config{
							APIKeys:     keys,
							OpenCodeGo:  config.OpenCodeGoConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint, ResponsesBaseURL: endpoint},
							OpenCodeZen: config.OpenCodeZenConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint, ResponsesBaseURL: endpoint, GeminiBaseURL: endpoint},
							AWSBedrock:  config.AWSBedrockConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint},
							OpenRouter:  config.OpenRouterConfig{BaseURL: endpoint},
							CommandCode: config.CommandCodeConfig{BaseURL: endpoint, AnthropicBaseURL: endpoint, APIKeys: keys},
							ClinePass:   config.ClinePassConfig{BaseURL: endpoint, APIKeys: keys},
						}
						atomic := config.NewAtomicConfig(cfg, "")
						var p core.Provider
						var pool *keyPool
						switch name {
						case "opencode-go":
							v := NewOpenCodeGoProvider(atomic, nil)
							p, pool = v, v.keys
						case "opencode-zen":
							v := NewOpenCodeZenProvider(atomic)
							p, pool = v, v.keys
						case "aws-bedrock":
							v := NewAWSBedrockProvider(atomic)
							p, pool = v, v.keys
						case "openrouter":
							v := NewOpenRouterProvider(atomic, nil)
							p, pool = v, v.keys
						case "commandcode":
							v := NewCommandCodeProvider(atomic, nil)
							p, pool = v, v.keys
						case "cline-pass":
							v := NewClinePassProvider(atomic, nil)
							p, pool = v, v.keys
						}
						model := config.ModelConfig{Provider: name, ModelID: "test", WireFormat: format.String()}
						_, _, req := cancellationTestProvider("")
						req.MaxTokens = 16
						ctx, cancel := context.WithCancel(context.Background())
						defer cancel()
						if failure == "canceled" {
							cancel()
						}
						var err error
						if stream {
							var body io.ReadCloser
							body, err = p.Stream(ctx, req, model)
							if body != nil {
								_ = body.Close()
							}
						} else {
							_, err = p.Execute(ctx, req, model)
						}
						if err == nil {
							t.Fatal("failure unexpectedly succeeded")
						}
						st := pool.states[keys[0]]
						if st == nil || st.inFlight != 0 {
							t.Fatal("reservation leaked")
						}
						if failure == "invalid_url" || failure == "canceled" {
							if !st.blockedUntil.IsZero() || st.consecutive != 0 {
								t.Fatal("local failure changed key health")
							}
						} else {
							var apiErr *client.APIError
							if !errors.As(err, &apiErr) {
								t.Fatalf("missing status: %v", err)
							}
							if pool.blockedUntil(keys[0]).Before(time.Now().Add(119 * time.Second)) {
								t.Fatal("missing auth quarantine or Retry-After")
							}
						}
					})
				}
			}
		}
	}
}
func fmtStream(stream bool) string {
	if stream {
		return "/stream"
	}
	return "/execute"
}
