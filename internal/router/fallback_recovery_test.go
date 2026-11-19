package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
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

func TestKeyCoolingSkipsProviderWithoutOpeningCircuit(t *testing.T) {
	h := NewFallbackHandler(nil, 1, time.Second)
	model := config.ModelConfig{Provider: "cline-pass", ModelID: "synthetic"}
	// The timestamp contains "500"; legacy text classification must not apply.
	err := &core.KeysCoolingError{RetryAt: time.Date(2026, 11, 15, 1, 2, 3, 500, time.UTC)}
	if IsRetryableError(err) {
		t.Fatal("local cooling penalizes the model")
	}
	if !h.AllowAttempt(model)(t.Context(), err) {
		t.Fatal("all keys cooling must skip the provider for this request")
	}
	if h.getCircuitBreaker(config.ModelKey(model)).State() != CircuitClosed {
		t.Fatal("local cooling opened the model circuit")
	}
}

func TestKeyCoolingSkipsSameProviderInFallback(t *testing.T) {
	for _, other := range []bool{false, true} {
		h := NewFallbackHandler(nil, 1, time.Second)
		models := []config.ModelConfig{
			{Provider: "cline-pass", ModelID: "one"},
			{Provider: "cline-pass", ModelID: "two"},
		}
		want := []string{"one"}
		if other {
			models = append(models, config.ModelConfig{Provider: "workbuddy", ModelID: "other"})
			want = append(want, "other")
		}
		cooling := &core.KeysCoolingError{RetryAt: time.Now().Add(time.Minute)}
		var calls []string
		_, _, err := h.ExecuteWithFallback(t.Context(), models, func(_ context.Context, model config.ModelConfig, _ int, _ int) ([]byte, error) {
			calls = append(calls, model.ModelID)
			if model.Provider == "cline-pass" {
				return nil, cooling
			}
			return []byte("ok"), nil
		})
		if !reflect.DeepEqual(calls, want) || (other && err != nil) || (!other && !errors.Is(err, cooling)) {
			t.Fatalf("calls=%v want=%v err=%v", calls, want, err)
		}
	}
}

func TestSourceHealthIgnoresJoinedDeliveryFailure(t *testing.T) {
	for _, source := range []struct {
		name string
		err  error
		open bool
	}{
		{"failed", errors.New("upstream Responses status: failed"), false},
		{"overload", &client.APIError{StatusCode: 503}, true},
		{"deadline", context.DeadlineExceeded, true},
		{"auth", &client.APIError{StatusCode: 401}, false},
	} {
		for _, delivery := range []error{nil, io.ErrClosedPipe, context.DeadlineExceeded} {
			for _, canceled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/delivery=%v/canceled=%t", source.name, delivery, canceled), func(t *testing.T) {
					h := NewFallbackHandler(nil, 1, time.Minute)
					model := config.ModelConfig{Provider: "synthetic", ModelID: "m"}
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					complete := h.AllowAttempt(model)
					if canceled {
						cancel()
					}
					complete(ctx, errors.Join(&core.ResponseError{Err: source.err}, delivery))
					if open := h.getCircuitBreaker(config.ModelKey(model)).State() == CircuitOpen; open != (source.open && !canceled) {
						t.Fatalf("open=%t source=%v delivery=%v canceled=%t", open, source.err, delivery, canceled)
					}
				})
			}
		}
	}
}
