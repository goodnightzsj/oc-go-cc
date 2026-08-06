package router

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"oc-go-cc/internal/client"
	"oc-go-cc/internal/config"
)

// newTestFallbackHandler returns a handler with the single-key auth predicate
// fixed to the given value, so tests can exercise single- and multi-key paths.
func newTestFallbackHandler(singleKey bool) *FallbackHandler {
	h := NewFallbackHandler(slog.Default(), 3, 30_000_000_000)
	h.SetAuthSingleKey(func() bool { return singleKey })
	return h
}

func TestExecuteWithFallback_SingleKeyAuthError_ShortCircuits(t *testing.T) {
	h := newTestFallbackHandler(true)

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "primary"},
		{Provider: "opencode-go", ModelID: "fallback1"},
		{Provider: "opencode-go", ModelID: "fallback2"},
	}
	attempts := 0
	executor := func(ctx context.Context, m config.ModelConfig) ([]byte, error) {
		attempts++
		// Simulate an invalid key: upstream rejects with HTTP 401 regardless
		// of which model is being invoked.
		return nil, &client.UpstreamError{StatusCode: 401}
	}

	_, _, err := h.ExecuteWithFallback(context.Background(), chain, executor)
	if err == nil {
		t.Fatal("expected an error for auth failure")
	}
	// Only the primary should have been attempted — the chain short-circuits
	// on the first auth error instead of visiting every fallback.
	if attempts != 1 {
		t.Errorf("short-circuit expected 1 attempt, got %d (fallbacks were tried)", attempts)
	}
}

func TestExecuteWithFallback_MultiKeyAuthError_DoesNotShortCircuit(t *testing.T) {
	// With multiple API keys, the next fallback model may rotate to a valid
	// key, so an auth error must NOT abort the chain.
	h := NewFallbackHandler(slog.Default(), 3, 30_000_000_000)
	h.SetAuthSingleKey(func() bool { return false }) // multi-key

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "primary"},
		{Provider: "opencode-go", ModelID: "fallback1"},
	}
	attempts := 0
	executor := func(ctx context.Context, m config.ModelConfig) ([]byte, error) {
		attempts++
		if m.ModelID == "fallback1" {
			return []byte("ok"), nil
		}
		return nil, &client.UpstreamError{StatusCode: 401}
	}

	result, body, err := h.ExecuteWithFallback(context.Background(), chain, executor)
	if err != nil {
		t.Fatalf("multi-key should continue to a working fallback: %v", err)
	}
	if result.ModelID != "fallback1" || string(body) != "ok" {
		t.Errorf("expected fallback1 to succeed, got model=%s body=%q", result.ModelID, body)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestExecuteWithFallback_NoAuthPredicate_NoShortCircuit(t *testing.T) {
	// Default handler (no predicate installed) must behave exactly as before:
	// an auth error still moves on to the next fallback.
	h := NewFallbackHandler(slog.Default(), 3, 30_000_000_000)

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "primary"},
		{Provider: "opencode-go", ModelID: "fallback1"},
	}
	attempts := 0
	executor := func(ctx context.Context, m config.ModelConfig) ([]byte, error) {
		attempts++
		if m.ModelID == "fallback1" {
			return []byte("ok"), nil
		}
		return nil, &client.UpstreamError{StatusCode: 401}
	}

	result, _, err := h.ExecuteWithFallback(context.Background(), chain, executor)
	if err != nil {
		t.Fatalf("default handler should not short-circuit: %v", err)
	}
	if result.ModelID != "fallback1" {
		t.Errorf("expected fallback1 to succeed, got %s", result.ModelID)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestExecuteWithFallback_NonAuthErrorNeverShortCircuits(t *testing.T) {
	// A 500 (server error) is retryable and must always try fallbacks, even in
	// single-key mode.
	h := newTestFallbackHandler(true)

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "primary"},
		{Provider: "opencode-go", ModelID: "fallback1"},
	}
	executor := func(ctx context.Context, m config.ModelConfig) ([]byte, error) {
		if m.ModelID == "fallback1" {
			return []byte("ok"), nil
		}
		return nil, &client.UpstreamError{StatusCode: 500}
	}

	result, _, err := h.ExecuteWithFallback(context.Background(), chain, executor)
	if err != nil {
		t.Fatalf("500 should not short-circuit single-key chain: %v", err)
	}
	if result.ModelID != "fallback1" {
		t.Errorf("expected fallback1 to succeed, got %s", result.ModelID)
	}
}

func TestIsAuthError_Detects401And403Only(t *testing.T) {
	h := NewFallbackHandler(slog.Default(), 3, 30_000_000_000)
	if !h.isAuthError(&client.UpstreamError{StatusCode: 401}) {
		t.Error("401 should be recognized as an auth error")
	}
	if !h.isAuthError(&client.UpstreamError{StatusCode: 403}) {
		t.Error("403 should be recognized as an auth error")
	}
	for _, code := range []int{400, 429, 500, 502} {
		if h.isAuthError(&client.UpstreamError{StatusCode: code}) {
			t.Errorf("%d should NOT be recognized as an auth error", code)
		}
	}
	// Non-UpstreamError values are never auth errors.
	if h.isAuthError(errors.New("boom")) || h.isAuthError(nil) {
		t.Error("non-upstream errors must not be treated as auth errors")
	}
}
