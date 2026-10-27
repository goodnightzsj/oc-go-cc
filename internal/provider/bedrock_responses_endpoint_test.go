package provider

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestBedrockResponsesUsesModelAwarePath(t *testing.T) {
	for _, tc := range []struct{ path, model, want string }{
		{"/v1/chat/completions", "openai.gpt-5.1", "/openai/v1/responses"},
		{"/openai/v1/chat/completions", "openai.gpt-5.1", "/openai/v1/responses"},
		{"/v1", "openai.gpt-5.1", "/openai/v1/responses"},
		{"/v1/", "openai.gpt-5.1", "/openai/v1/responses"},
		{"/openai/v1/responses", "openai.gpt-5.1", "/openai/v1/responses"},
		{"/v1/chat/completions", "zai.glm-5", "/v1/responses"},
		{"/gateway/v1/chat/completions", "openai.gpt-5.1", "/gateway/openai/v1/responses"},
		{"/custom", "openai.gpt-5.1", "/custom/responses"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(tc.path+"/"+tc.model+map[bool]string{true: "/stream", false: "/json"}[stream], func(t *testing.T) {
				cfg := config.NewAtomicConfig(&config.Config{AWSBedrock: config.AWSBedrockConfig{APIKey: "synthetic", BaseURL: "https://fixture.invalid" + tc.path}}, "")
				p := NewAWSBedrockProvider(cfg)
				path := ""
				p.httpClient = &http.Client{Transport: responseContractTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Host != "fixture.invalid" {
						t.Errorf("host changed: %s", r.URL.Host)
					}
					path = r.URL.Path
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`)), Request: r}, nil
				})}
				model := config.ModelConfig{Provider: p.Name(), ModelID: tc.model, WireFormat: "responses"}
				req := &types.MessageRequest{MaxTokens: 128}
				if stream {
					body, err := p.Stream(t.Context(), req, model)
					if err != nil {
						t.Fatal(err)
					}
					if err := body.Close(); err != nil {
						t.Fatal(err)
					}
				} else if _, err := p.Execute(t.Context(), req, model); err != nil {
					t.Fatal(err)
				}
				if path != tc.want {
					t.Fatalf("path=%s want=%s", path, tc.want)
				}
			})
		}
	}
}
