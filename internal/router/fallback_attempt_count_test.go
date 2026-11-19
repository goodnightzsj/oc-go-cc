package router

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

func TestFallbackCountsExecutionsNotSkippedCandidates(t *testing.T) {
	for _, alternate := range []bool{false, true} {
		h := NewFallbackHandler(nil, 1, time.Minute)
		chain := []config.ModelConfig{
			{Provider: "cline-pass", ModelID: "cline-pass/one"},
			{Provider: "cline-pass", ModelID: "cline-pass/two"},
			{Provider: "cline-pass", ModelID: "cline-pass/three"},
			{Provider: "cline-pass", ModelID: "cline-pass/four"},
		}
		if alternate {
			chain = append(chain, config.ModelConfig{Provider: "workbuddy", ModelID: "other"})
		}
		cooling := &core.KeysCoolingError{RetryAt: time.Now().Add(time.Minute)}
		calls := 0
		result, _, err := h.ExecuteWithFallback(t.Context(), chain, func(_ context.Context, model config.ModelConfig, attempt, candidate int) ([]byte, error) {
			calls++
			if attempt != calls {
				t.Errorf("callback attempt=%d actual execution=%d", attempt, calls)
			}
			if candidate != 1 && candidate != 5 {
				t.Errorf("callback candidate=%d, want original chain position 1 or 5", candidate)
			}
			if model.Provider == "cline-pass" {
				return nil, cooling
			}
			return []byte("ok"), nil
		})
		if result.Attempted != calls || (!alternate && (!errors.Is(err, cooling) || strings.Contains(err.Error(), "4 attempts"))) || (alternate && err != nil) {
			t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
		}
		if alternate && result.CandidatePosition != 5 {
			t.Fatalf("successful fallback lost candidate identity: %+v", result)
		}
	}
}

func TestFallbackAllCircuitsOpenCountsNoExecution(t *testing.T) {
	h := NewFallbackHandler(nil, 1, time.Minute)
	chain := []config.ModelConfig{{Provider: "synthetic", ModelID: "one"}, {Provider: "synthetic", ModelID: "two"}}
	for _, model := range chain {
		h.AllowAttempt(model)(t.Context(), errors.New("synthetic HTTP 503"))
	}
	result, _, err := h.ExecuteWithFallback(t.Context(), chain, func(context.Context, config.ModelConfig, int, int) ([]byte, error) {
		t.Fatal("open circuit called executor")
		return nil, nil
	})
	if result.Attempted != 0 || !errors.Is(err, ErrAllModelsFailed) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
