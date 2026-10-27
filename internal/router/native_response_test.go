package router

import (
	"context"
	"errors"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

func TestCompleteNativeFailureStopsFallback(t *testing.T) {
	h := NewFallbackHandler(nil, 1, 0)
	models := []config.ModelConfig{{Provider: "commandcode", ModelID: "first"}, {Provider: "commandcode", ModelID: "second"}}
	want := &core.NativeResponseError{ResponseError: core.ResponseError{Err: errors.New("source failed")}, Response: &core.ExecuteResult{Body: []byte(`{"status":"failed"}`)}}
	calls := 0
	result, body, err := h.ExecuteWithFallback(t.Context(), models, func(context.Context, config.ModelConfig, int) ([]byte, error) {
		calls++
		return want.FinalResponse(), want
	})
	if !errors.Is(err, want) || calls != 1 || result.ModelID != "first" || result.Attempted != 1 || string(body) != string(want.FinalResponse()) {
		t.Fatalf("failure was replayed/hidden: result=%+v calls=%d body=%s err=%v", result, calls, body, err)
	}
}
