package router

import (
	"context"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

func TestHalfOpenNeutralCompletionAndGeneration(t *testing.T) {
	ctx := context.Background()
	model := config.ModelConfig{ModelID: "synthetic"}
	h := NewFallbackHandler(nil, 1, time.Second)
	cb := h.getCircuitBreaker(config.ModelKey(model))
	age := func() {
		cb.mu.Lock()
		cb.lastFailureTime = time.Now().Add(-time.Minute)
		cb.mu.Unlock()
	}
	h.AllowAttempt(model)(ctx, &client.APIError{StatusCode: 503})
	age()
	for _, status := range []int{400, 401, 403, 429} {
		complete := h.AllowAttempt(model)
		if complete == nil {
			t.Fatal("neutral result exhausted probes")
		}
		complete(ctx, &client.APIError{StatusCode: status})
		complete(ctx, nil) // Duplicate completion must not vote twice.
	}
	canceled, cancel := context.WithCancel(ctx)
	complete := h.AllowAttempt(model)
	cancel()
	complete(canceled, context.Canceled)
	if cb.State() != CircuitHalfOpen {
		t.Fatal("neutral result changed health")
	}
	old := h.AllowAttempt(model)
	failed := h.AllowAttempt(model)
	third := h.AllowAttempt(model)
	if old == nil || failed == nil || third == nil || h.AllowAttempt(model) != nil {
		t.Fatal("half-open concurrency limit not enforced")
	}
	failed(ctx, &client.APIError{StatusCode: 503})
	age()
	first := h.AllowAttempt(model)
	second := h.AllowAttempt(model)
	last := h.AllowAttempt(model)
	old(ctx, nil)
	third(ctx, &client.APIError{StatusCode: 503})
	if cb.State() != CircuitHalfOpen || h.AllowAttempt(model) != nil {
		t.Fatal("old completion affected new recovery round")
	}
	first(ctx, nil)
	second(ctx, nil)
	if cb.State() != CircuitHalfOpen {
		t.Fatal("closed before three successes")
	}
	last(ctx, nil)
	if cb.State() != CircuitClosed {
		t.Fatal("healthy probes did not close circuit")
	}
}
