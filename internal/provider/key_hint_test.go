package provider

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

func TestOpenCodeGoKeyHintAcrossFormatsAndCapture(t *testing.T) {
	for format, response := range map[string]string{
		"openai":    `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`,
		"responses": `{"status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`,
		"anthropic": `{"type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`,
	} {
		for _, stream := range []bool{false, true} {
			for _, captureEnabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/stream=%t/capture=%t", format, stream, captureEnabled), func(t *testing.T) {
					p := responseContractProviders(response)[0].(*OpenCodeGoProvider)
					p.atomic.Get().OpenCodeGo.AnthropicBaseURL = "https://synthetic.invalid/messages"
					if captureEnabled {
						store, err := debug.NewStorage(config.DebugCapture{Enabled: true, Directory: filepath.Join(t.TempDir(), "capture"), MaxFiles: 2, MaxFileSize: 1 << 20})
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = store.Close() })
						p.capture = debug.NewCaptureLogger(store, true)
						t.Cleanup(func() { _ = p.capture.Close() })
					}
					req := &types.MessageRequest{MaxTokens: 16}
					model := config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: format}
					var hint string
					if stream {
						body, err := p.Stream(t.Context(), req, model)
						if err != nil {
							t.Fatal(err)
						}
						defer body.Close()
						if _, err := io.Copy(io.Discard, body); err != nil {
							t.Fatal(err)
						}
						hint = client.KeyHintOf(body)
					} else {
						result, err := p.Execute(t.Context(), req, model)
						if err != nil {
							t.Fatal(err)
						}
						hint = result.KeyHint
					}
					if hint != "****etic" {
						t.Fatalf("key hint = %q, want masked credential label", hint)
					}
				})
			}
		}
	}
}

// The label has to survive every wrapper between the provider and the handler.
// Each one erases a method it does not declare, which this project has hit five
// times already.
func TestKeyHintSurvivesEveryWrapper(t *testing.T) {
	const hint = "****abcd"
	base := &keyHintBody{ReadCloser: io.NopCloser(strings.NewReader("x")), hint: hint}

	if got := client.KeyHintOf(base); got != hint {
		t.Fatalf("bare body: %q", got)
	}

	peeked := &peekedBody{ReadCloser: base}
	if got := client.KeyHintOf(peeked); got != hint {
		t.Errorf("peekedBody erased the hint: %q", got)
	}

	closed := &cancelOnClose{ReadCloser: peeked, cancel: func() {}}
	if got := client.KeyHintOf(closed); got != hint {
		t.Errorf("cancelOnClose erased the hint: %q", got)
	}

	ctxBody := transformer.NewCtxReadCloser(t.Context(), closed)
	if got := client.KeyHintOf(ctxBody); got != hint {
		t.Errorf("ctxReadCloser erased the hint: %q", got)
	}

	captured := client.CaptureBody(ctxBody, func([]byte) {})
	if got := client.KeyHintOf(captured); got != hint {
		t.Errorf("captureReadCloser erased the hint: %q", got)
	}
}

// A short or empty key must not leak more than a label.
func TestKeyHintNeverShowsAWholeShortKey(t *testing.T) {
	p := newKeyPool()
	for _, key := range []string{"a", "abc", "abcd"} {
		if got := p.Reserve([]string{key}).KeyHint(); strings.Contains(got, key) {
			t.Errorf("hint %q contains the whole key %q", got, key)
		}
	}
	sel := p.Reserve([]string{"sk-12345678"})
	if got := sel.KeyHint(); got != "****5678" {
		t.Errorf("hint = %q, want the last four only", got)
	}
	if strings.Contains(sel.KeyHint(), "sk-1") {
		t.Error("hint leaked more than the last four characters")
	}
}
