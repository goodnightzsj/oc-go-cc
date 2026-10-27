package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// Reasoning tokens are a breakdown of the completion, not an extra charge. The
// column must round-trip so a gateway that reports the split (WorkBuddy answers
// with completion_tokens_details.reasoning_tokens) does not lose it, and so a
// reader can tell how much of the answer was thinking.
func TestReasoningTokensRoundTrip(t *testing.T) {
	db := newCostTestDB(t)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "reasoning-1", Model: "m", Provider: "p",
		StartTime:    time.Now().Add(-time.Minute),
		OutputTokens: 36, ReasoningTokens: 26, Success: true,
	})

	rows, _, err := NewRequests(db).Query(t.Context(), RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].ReasoningTokens != 26 {
		t.Errorf("ReasoningTokens = %d, want 26", rows[0].ReasoningTokens)
	}
	// It is part of the output, never additional to it.
	if rows[0].OutputTokens != 36 {
		t.Errorf("OutputTokens = %d, want 36 unchanged by the split", rows[0].OutputTokens)
	}
}

// A platform that reports no split stores zero, and that zero is not an error.
func TestReasoningTokensAbsentIsZero(t *testing.T) {
	db := newCostTestDB(t)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "no-reasoning", Model: "m", Provider: "p",
		StartTime: time.Now().Add(-time.Minute), OutputTokens: 10, Success: true,
	})
	rows, _, err := NewRequests(db).Query(t.Context(), RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].ReasoningTokens != 0 {
		t.Errorf("ReasoningTokens = %d, want 0", rows[0].ReasoningTokens)
	}
}
