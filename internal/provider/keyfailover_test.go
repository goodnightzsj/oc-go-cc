package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// This is the integration the unit tests cannot see: whether a provider
// actually feeds the upstream's answer back into the key scheduler. The pool
// itself is covered in key_rotation_test.go; here the question is whether a
// rejected key is reported at all, because a scheduler that is never told
// anything behaves exactly like round-robin.

// The first key is rejected by the upstream and the second is accepted. The
// request must succeed on the second, and every later request must keep using
// it while the first is quarantined - the whole point of the change.
func TestProviderFailsOverToTheNextKeyAndStaysThere(t *testing.T) {
	var sawKey atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		sawKey.Store(auth)
		if auth == "Bearer rejected-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"bad key"}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	cfg := &config.Config{OpenRouter: config.OpenRouterConfig{
		BaseURL: upstream.URL, APIKey: "rejected-key", APIKeys: []string{"good-key"},
	}}
	p := NewOpenRouterProvider(config.NewAtomicConfig(cfg, ""), nil)
	model := config.ModelConfig{Provider: p.Name(), ModelID: "anthropic/claude-test"}
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
	ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{})

	// First attempt lands on the rejected key and fails.
	if _, err := p.Execute(ctx, req, model); err == nil {
		t.Fatal("expected the first attempt to fail on the rejected key")
	}
	if got, _ := sawKey.Load().(string); got != "Bearer rejected-key" {
		t.Fatalf("first attempt used %q, want the first configured key", got)
	}

	// The next attempt must move to the good key, and stay there.
	for i := 0; i < 3; i++ {
		if _, err := p.Execute(ctx, req, model); err != nil {
			t.Fatalf("attempt %d after failover failed: %v", i+2, err)
		}
		if got, _ := sawKey.Load().(string); got != "Bearer good-key" {
			t.Fatalf("attempt %d used %q, want the second key; a quarantined key must not come back on its own", i+2, got)
		}
	}
}

// A success must be reported too: without it a key that recovered would stay
// quarantined for the full window, and the scheduler could never learn that a
// transient failure was transient.
func TestProviderReportsSuccessToTheScheduler(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer upstream.Close()

	cfg := &config.Config{OpenRouter: config.OpenRouterConfig{BaseURL: upstream.URL, APIKey: "k1", APIKeys: []string{"k2"}}}
	p := NewOpenRouterProvider(config.NewAtomicConfig(cfg, ""), nil)
	model := config.ModelConfig{Provider: p.Name(), ModelID: "anthropic/claude-test"}
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}}
	ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{})

	if _, err := p.Execute(ctx, req, model); err != nil {
		t.Fatal(err)
	}
	// After a success the first key carries no failures, so a transient blip on
	// it must not have accumulated toward a cooldown.
	if st := p.keys.states["k1"]; st != nil && (st.consecutive != 0 || !st.blockedUntil.IsZero()) {
		t.Fatalf("a successful request left state behind: %+v", st)
	}
}
