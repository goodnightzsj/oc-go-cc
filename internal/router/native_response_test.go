package router

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

func TestCompleteNativeFailureStopsFallback(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		for _, skippedPrimary := range []bool{false, true} {
			t.Run(fmt.Sprintf("canceled=%t/skipped_primary=%t", canceled, skippedPrimary), func(t *testing.T) {
				h := NewFallbackHandler(nil, 1, time.Minute)
				models := []config.ModelConfig{{Provider: "commandcode", ModelID: "first"}, {Provider: "commandcode", ModelID: "second"}, {Provider: "commandcode", ModelID: "third"}}
				position := 1
				if skippedPrimary {
					h.AllowAttempt(models[0])(t.Context(), errors.New("synthetic HTTP 503"))
					position = 2
				}
				want := &core.NativeResponseError{ResponseError: core.ResponseError{Err: &client.APIError{StatusCode: 503}}, Response: &core.ExecuteResult{Body: []byte(`{"status":"failed"}`)}}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				calls := 0
				result, body, err := h.ExecuteWithFallback(ctx, models, func(context.Context, config.ModelConfig, int, int) ([]byte, error) {
					calls++
					if canceled {
						cancel()
					}
					return want.FinalResponse(), want
				})
				if !errors.Is(err, want) || calls != 1 || result == nil || result.ModelID != models[position-1].ModelID || result.Provider != "commandcode" || result.Attempted != 1 || result.CandidatePosition != position || string(body) != string(want.FinalResponse()) {
					t.Fatalf("failure was replayed/hidden: result=%+v calls=%d body=%s err=%v", result, calls, body, err)
				}
				wantState := CircuitOpen
				if canceled {
					wantState = CircuitClosed
				}
				if state := h.getCircuitBreaker(config.ModelKey(models[position-1])).State(); state != wantState {
					t.Fatalf("source health/cancellation ownership changed: state=%v want=%v", state, wantState)
				}
			})
		}
	}
}
