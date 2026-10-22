package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/pkg/types"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestClientCancellationMustNotCoolKeys(t *testing.T) {
	entered := make(chan struct{})
	var serveOK atomic.Bool
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		if serveOK.Load() {
			_, _ = io.WriteString(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
			return
		}
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer upstream.Close()
	p, model, req := cancellationTestProvider(upstream.URL)
	for i := 0; i < 6; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() { _, err := p.Execute(ctx, req, model); result <- err }()
		<-entered
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("attempt %d error = %v, want context.Canceled", i, err)
		}
	}
	serveOK.Store(true)
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("healthy request rejected after %d client cancellations (upstream calls=%d): %v", 6, calls.Load(), err)
	}
}

func cancellationTestProvider(endpoint string) (*OpenRouterProvider, config.ModelConfig, *types.MessageRequest) {
	cfg := &config.Config{OpenRouter: config.OpenRouterConfig{BaseURL: endpoint, APIKeys: []string{"synthetic-a", "synthetic-b"}}}
	p := NewOpenRouterProvider(config.NewAtomicConfig(cfg, ""), nil)
	model := config.ModelConfig{Provider: p.Name(), ModelID: "anthropic/claude-test"}
	req := &types.MessageRequest{Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hello"`)}}}
	return p, model, req
}

func TestSelectionCancellationIsNeutral(t *testing.T) {
	p := newKeyPool()
	held := p.Reserve([]string{"a"})
	p.Reserve([]string{"a"}).Settle(&client.APIError{StatusCode: http.StatusUnauthorized})
	blocked := p.blockedUntil("a")
	held.Settle(fmt.Errorf("request: %w", context.Canceled))
	if p.blockedUntil("a") != blocked || p.states["a"].inFlight != 0 || p.states["a"].consecutive != 0 {
		t.Fatal("cancellation changed key health or leaked reservation")
	}
	p = newKeyPool()
	for i := 0; i < transientThreshold; i++ {
		p.Reserve([]string{"a"}).Settle(context.DeadlineExceeded)
	}
	if p.blockedUntil("a").IsZero() {
		t.Fatal("real upstream timeouts must still cool the key")
	}
}
