package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

// The history row must say which credential carried the request. Capture is on
// in production, and captureReadCloser forwards neither KeyHint nor FirstByteAt,
// so wrapping in the wrong order silently empties both - the row still looks
// well-formed, which is why this needs a test rather than a comment.
func TestWorkBuddyReportsKeyHintAndFirstByteThroughCapture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A delay before the first byte: the stamp must land on the read, not
		// on the response headers.
		time.Sleep(30 * time.Millisecond)
		_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	p, cfg := newWorkBuddyTestProvider(t)
	cfg.WorkBuddy.BaseURL = srv.URL
	cfg.WorkBuddy.APIKey = "sk-test-abcd"
	p.atomic.ApplyLoaded(cfg)

	start := time.Now()
	body, err := p.Stream(context.Background(), workBuddyTestRequest(),
		config.ModelConfig{Provider: "workbuddy", ModelID: "global:x"})
	if err != nil {
		t.Fatal(err)
	}
	// FirstByteAt must be zero until bytes actually arrive.
	if at := client.FirstByteAtOf(body); !at.IsZero() {
		t.Errorf("first byte stamped before any read: %v", at)
	}
	if _, err := io.Copy(io.Discard, body); err != nil {
		t.Fatal(err)
	}
	_ = body.Close()

	if hint := client.KeyHintOf(body); hint == "" {
		t.Error("KeyHint is empty; the history row would record no credential")
	} else if strings.Contains(hint, "sk-test-abcd") {
		t.Errorf("KeyHint %q contains the credential itself", hint)
	}
	at := client.FirstByteAtOf(body)
	if at.IsZero() {
		t.Fatal("FirstByteAt is zero; first_byte_ms would be NULL")
	}
	if at.Before(start) {
		t.Errorf("first byte %v predates the request %v", at, start)
	}
	if at.Sub(start) < 20*time.Millisecond {
		t.Errorf("first byte %v is too close to the start %v; the stamp looks taken at the headers, not the read", at, start)
	}
}

// A non-streaming call has a first byte too, and the same body type serves it.
func TestWorkBuddyExecuteReportsFirstByte(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`)
	}))
	defer srv.Close()

	p, cfg := newWorkBuddyTestProvider(t)
	cfg.WorkBuddy.BaseURL = srv.URL
	cfg.WorkBuddy.APIKey = "k"
	p.atomic.ApplyLoaded(cfg)

	res, err := p.Execute(context.Background(), workBuddyTestRequest(),
		config.ModelConfig{Provider: "workbuddy", ModelID: "global:x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.KeyHint == "" {
		t.Error("Execute returned no KeyHint")
	}
}
