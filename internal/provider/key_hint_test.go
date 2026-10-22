package provider

import (
	"io"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/transformer"
)

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
