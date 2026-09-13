package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestGoExplicitResponsesEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/responses" || payload["input"] == nil || payload["messages"] != nil {
			t.Errorf("wire format/endpoint mismatch: %s %v", r.URL.Path, payload)
		}
		if string(payload["stream"]) == "true" {
			_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":2}}}\n\n")
		} else {
			_, _ = io.WriteString(w, `{"id":"resp-test","object":"response","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":4,"output_tokens":2}}`)
		}
	}))
	defer upstream.Close()
	cfg := &config.Config{OpenCodeGo: config.OpenCodeGoConfig{APIKey: "go-only", BaseURL: upstream.URL + "/chat", ResponsesBaseURL: upstream.URL + "/responses"}}
	p := NewOpenCodeGoProvider(config.NewAtomicConfig(cfg, ""), nil)
	model := config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "responses"}
	req := &types.MessageRequest{Model: "alias", Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatal(err)
	}
	body, err := p.Stream(context.Background(), req, model)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	if _, err := io.Copy(io.Discard, body); err != nil {
		t.Fatal(err)
	}
}

func TestProviderScopedHeaders(t *testing.T) {
	for _, name := range []string{"opencode-go", "opencode-zen", "aws-bedrock", "commandcode"} {
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantUA := "routatic-proxy"
				if name == "opencode-go" || name == "opencode-zen" {
					wantUA = "opencode/routatic-proxy"
				}
				if r.Header.Get("User-Agent") != wantUA {
					t.Errorf("wrong attribution: %q", r.Header.Get("User-Agent"))
				}
				wantSession := ""
				if name == "opencode-go" {
					wantSession = "synthetic-conversation"
				}
				if r.Header.Get("x-opencode-session") != wantSession {
					t.Error("session header missing or leaked across platforms")
				}
				if r.Header.Get("Authorization") != "Bearer dedicated-key" {
					t.Error("dedicated credential not used")
				}
				_, _ = io.WriteString(w, `{"id":"chat-test","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`)
			}))
			defer upstream.Close()
			cfg := &config.Config{APIKey: "global-key", OpenCodeGo: config.OpenCodeGoConfig{APIKey: "dedicated-key", BaseURL: upstream.URL}, OpenCodeZen: config.OpenCodeZenConfig{APIKey: "dedicated-key", BaseURL: upstream.URL}, AWSBedrock: config.AWSBedrockConfig{APIKey: "dedicated-key", BaseURL: upstream.URL}, CommandCode: config.CommandCodeConfig{APIKey: "dedicated-key", BaseURL: upstream.URL}}
			atomic := config.NewAtomicConfig(cfg, "")
			providers := map[string]core.Provider{"opencode-go": NewOpenCodeGoProvider(atomic, nil), "opencode-zen": NewOpenCodeZenProvider(atomic), "aws-bedrock": NewAWSBedrockProvider(atomic), "commandcode": NewCommandCodeProvider(atomic, nil)}
			ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{SessionID: "synthetic-conversation"})
			_, err := providers[name].Execute(ctx, &types.MessageRequest{Model: "alias", Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}, config.ModelConfig{Provider: name, ModelID: "synthetic"})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
