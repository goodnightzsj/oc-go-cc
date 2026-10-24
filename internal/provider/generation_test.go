package provider

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/metrics"
)

type generationProviderBody struct {
	chunks []string
	closed bool
}

func (b *generationProviderBody) Read(p []byte) (int, error) {
	if len(b.chunks) == 0 {
		time.Sleep(5 * time.Second)
		return 0, io.EOF
	}
	time.Sleep(time.Second)
	n := copy(p, b.chunks[0])
	b.chunks[0] = b.chunks[0][n:]
	if b.chunks[0] == "" {
		b.chunks = b.chunks[1:]
	}
	return n, nil
}
func (b *generationProviderBody) Close() error { b.closed = true; return nil }

func TestGenerationBeforeProviderPeekAndCapture(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("capture=%t", enabled), func(t *testing.T) {
			dir := t.TempDir()
			store, err := debug.NewStorage(config.DebugCapture{Enabled: enabled, Directory: dir, MaxFileSize: 1 << 20, MaxFiles: 2, RedactAPIKeys: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			capture := debug.NewCaptureLogger(store, enabled)
			t.Cleanup(func() {
				if err := capture.Close(); err != nil {
					t.Error(err)
				}
			})
			cfg := config.NewAtomicConfig(&config.Config{
				OpenCodeGo:  config.OpenCodeGoConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
				OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
				AWSBedrock:  config.AWSBedrockConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/v1"},
				OpenRouter:  config.OpenRouterConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
				CommandCode: config.CommandCodeConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
				WorkBuddy:   config.WorkBuddyConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
				ClinePass:   config.ClinePassConfig{APIKey: "synthetic", BaseURL: "https://synthetic.invalid/chat"},
			}, "")
			goProvider, zen := NewOpenCodeGoProvider(cfg, capture), NewOpenCodeZenProvider(cfg)
			bedrock, openrouter := NewAWSBedrockProvider(cfg), NewOpenRouterProvider(cfg, capture)
			command, workbuddy := NewCommandCodeProvider(cfg, capture), NewWorkBuddyProvider(cfg, capture)
			cline := NewClinePassProvider(cfg, capture)
			for _, tc := range []struct {
				p    core.Provider
				base *baseProvider
			}{
				{goProvider, &goProvider.baseProvider}, {zen, &zen.baseProvider}, {bedrock, &bedrock.baseProvider},
				{openrouter, &openrouter.baseProvider}, {command, &command.baseProvider}, {workbuddy, &workbuddy.baseProvider}, {cline, &cline.baseProvider},
			} {
				t.Run(tc.p.Name(), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						chunks := []string{
							"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n",
							"data: {\"model\":\"served\",\"choices\":[{\"delta\":{}}]}\n\n",
							"data: {\"choices\":[],\"usage\":{\"completion_tokens\":20}}\n\ndata: [DONE]\n\n",
						}
						upstream := &generationProviderBody{chunks: append([]string(nil), chunks...)}
						tc.base.httpClient = &http.Client{Transport: responseContractTransport(func(r *http.Request) (*http.Response, error) {
							return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: upstream, Request: r}, nil
						})}
						start := time.Now()
						ctx, observation := metrics.WithGenerationObservation(t.Context(), start, core.WireFormatOpenAIChat)
						body, err := tc.p.Stream(ctx, clinePassRequest(), config.ModelConfig{Provider: tc.p.Name(), ModelID: "synthetic", WireFormat: "openai"})
						if err != nil {
							t.Fatal(err)
						}
						if tc.p.Name() == "cline-pass" && (client.ServedModelOf(body) != "served" || time.Since(start) != 2*time.Second) {
							t.Fatal("fixture did not exercise provider pre-read")
						}
						got, err := io.ReadAll(body)
						if err != nil || string(got) != strings.Join(chunks, "") {
							t.Fatal("stream bytes changed", err)
						}
						if err := body.Close(); err != nil || !upstream.closed {
							t.Fatal("close changed", err)
						}
						if sample := observation.Sample(); !reflect.DeepEqual(sample, &metrics.GenerationSample{FirstOutputMs: 1000, DurationMs: 2000, OutputTokens: 20}) {
							t.Fatalf("sample=%+v", sample)
						}
					})
				})
			}
		})
	}
}
