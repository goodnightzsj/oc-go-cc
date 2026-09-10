package router

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

func TestFallbackSeparatesProvidersWithSameModel(t *testing.T) {
	h := NewFallbackHandler(nil, 1, time.Minute)
	chain := []config.ModelConfig{
		{Provider: "opencode_go", ModelID: "same-model"},
		{Provider: "opencode-zen", ModelID: "same-model"},
	}
	for i := 0; i < 2; i++ {
		result, body, err := h.ExecuteWithFallback(context.Background(), chain, func(_ context.Context, model config.ModelConfig) ([]byte, error) {
			if config.NormalizeProvider(model.Provider) == "opencode-go" {
				return nil, &client.APIError{StatusCode: http.StatusServiceUnavailable}
			}
			return []byte("zen"), nil
		})
		if err != nil || result.Provider != "opencode-zen" || string(body) != "zen" {
			t.Fatalf("healthy provider skipped or misattributed: result=%+v body=%q err=%v", result, body, err)
		}
	}
	states := h.GetCircuitStates()
	if states["opencode-go/same-model"] != "open" || states["opencode-zen/same-model"] != "closed" {
		t.Fatalf("provider circuit states = %v", states)
	}
}

func TestFallbackEmptyChainReturnsError(t *testing.T) {
	h := NewFallbackHandler(nil, 1, time.Minute)
	if _, _, err := h.ExecuteWithFallback(context.Background(), nil, nil); err == nil {
		t.Fatal("empty chain must fail without panicking")
	}
}
