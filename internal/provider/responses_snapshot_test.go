package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

func TestNativeResponsesReplayUsesCheckedSourceAcrossReload(t *testing.T) {
	for name, factory := range map[string]func(*config.AtomicConfig) core.Provider{
		"opencode-go":  func(c *config.AtomicConfig) core.Provider { return NewOpenCodeGoProvider(c, nil) },
		"opencode-zen": func(c *config.AtomicConfig) core.Provider { return NewOpenCodeZenProvider(c) },
		"aws-bedrock":  func(c *config.AtomicConfig) core.Provider { return NewAWSBedrockProvider(c) },
		"commandcode":  func(c *config.AtomicConfig) core.Provider { return NewCommandCodeProvider(c, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer synthetic-original" {
					t.Error("replay changed credential")
				}
				if name == "aws-bedrock" && r.Header.Get("OpenAI-Project") != "original-project" {
					t.Error("replay changed project")
				}
				if name == "commandcode" && r.Header.Get("x-cmd-zdr") != "1" {
					t.Error("replay changed retention")
				}
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, `{"status":"completed","output":[]}`)
			}))
			defer source.Close()
			original := &config.Config{
				OpenCodeGo:  config.OpenCodeGoConfig{APIKey: "synthetic-original", ResponsesBaseURL: source.URL},
				OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic-original", ResponsesBaseURL: source.URL},
				AWSBedrock:  config.AWSBedrockConfig{APIKey: "synthetic-original", BaseURL: source.URL, ProjectID: "original-project"},
				CommandCode: config.CommandCodeConfig{APIKey: "synthetic-original", ResponsesBaseURL: source.URL, ZeroDataRetention: true},
			}
			atomic := config.NewAtomicConfig(original, "")
			p := factory(atomic)
			ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{ResponsesBody: []byte(`{"model":"synthetic","input":[{"type":"reasoning","encrypted_content":"synthetic","summary":[]}]}`), ResponsesReplay: true, ResponsesSource: original})
			atomic.ApplyLoaded(&config.Config{})
			model := config.ModelConfig{Provider: name, ModelID: "synthetic", WireFormat: "responses"}
			if _, err := p.Execute(ctx, commandCodeRequest(), model); err != nil {
				t.Fatal(err)
			}
			body, err := p.Stream(ctx, commandCodeRequest(), model)
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(io.Discard, body)
			_ = body.Close()
			if err != nil || calls != 2 {
				t.Fatalf("checked source not used: calls=%d err=%v", calls, err)
			}
		})
	}
}
