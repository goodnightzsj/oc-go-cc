package gui

import (
	"testing"

	"github.com/routatic/proxy/internal/history"
)

func TestHistoryEntriesExposeRawAndPromptTokens(t *testing.T) {
	entries := toHistoryEntries([]history.RequestRecord{{
		InputTokens:         10,
		CacheReadTokens:     20,
		CacheCreationTokens: 30,
		OutputTokens:        40,
	}})
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
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
}
