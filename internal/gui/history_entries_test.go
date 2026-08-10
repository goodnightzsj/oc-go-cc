package gui

import (
	"testing"

	"github.com/routatic/proxy/internal/history"
)

func TestHistoryEntriesExposeRawAndPromptTokens(t *testing.T) {
	entries := toHistoryEntries([]history.RequestRecord{{
		ID:                  "request-1",
		InputTokens:         10,
		CacheReadTokens:     20,
		CacheCreationTokens: 30,
		CostUSD:             0.000123,
		OutputTokens:        40,
		Attempt:             2,
		ErrorMsg:            "upstream failed",
	}, {ID: "unknown-cost"}})
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	got := entries[0]
	if got.InputTokens != 10 {
		t.Errorf("input_tokens = %d, want raw input 10", got.InputTokens)
	}
	if got.PromptTokens != 60 {
		t.Errorf("prompt_tokens = %d, want raw+cache total 60", got.PromptTokens)
	}
	if got.CacheReadTokens != 20 || got.CacheCreationTokens != 30 {
		t.Errorf("cache tokens = (%d, %d), want (20, 30)", got.CacheReadTokens, got.CacheCreationTokens)
	}
	if got.CostUSD == nil || *got.CostUSD != 0.000123 {
		t.Errorf("cost_usd = %v, want 0.000123", got.CostUSD)
	}
	if entries[1].CostUSD != nil {
		t.Errorf("unknown cost_usd = %v, want nil", entries[1].CostUSD)
	}
	if got.ID != "request-1" || got.Attempt != 2 || got.ErrorMsg != "upstream failed" {
		t.Errorf("request detail fields = %+v, want id/attempt/error preserved", got)
	}
}
