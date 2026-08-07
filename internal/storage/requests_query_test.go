package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestRequestsQueryFiltersAndSortsFullDataset(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	base := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)

	for _, rec := range []history.RequestRecord{
		{ID: "r1", Model: "model-a", Provider: "provider-a", Scenario: "default", StartTime: base.Add(time.Hour), InputTokens: 10, CacheReadTokens: 20, Duration: 100 * time.Millisecond, Success: true},
		{ID: "r2", Model: "model-b", Provider: "provider-a", Scenario: "complex", StartTime: base.Add(2 * time.Hour), InputTokens: 5, OutputTokens: 30, Duration: 300 * time.Millisecond, Streaming: true, Success: false, ErrorMsg: "quota exceeded"},
		{ID: "r3", Model: "model-b", Provider: "provider-b", Scenario: "complex", StartTime: base.Add(3 * time.Hour), InputTokens: 100, OutputTokens: 10, Streaming: true, Success: true},
		{ID: "r4", Model: "model-c", Provider: "provider-b", Scenario: "default", StartTime: base.Add(48 * time.Hour), Success: true},
	} {
		if err := repo.Insert(rec); err != nil {
			t.Fatalf("insert %s: %v", rec.ID, err)
		}
	}

	failed := false
	rows, total, err := repo.Query(RequestQuery{
		Page: 1, PageSize: 50, Search: "quota", Success: &failed,
		Start: &base, End: timePtr(base.Add(24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("filtered query: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "r2" {
		t.Fatalf("filtered rows = %+v, total = %d; want only r2", rows, total)
	}

	streaming := true
	rows, total, err = repo.Query(RequestQuery{
		Page: 1, PageSize: 2, Start: &base, End: timePtr(base.Add(24 * time.Hour)),
		Streaming: &streaming, SortBy: "prompt_tokens", SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("sorted query: %v", err)
	}
	if total != 2 || len(rows) != 2 || rows[0].ID != "r2" || rows[1].ID != "r3" {
		t.Fatalf("sorted IDs = [%s %s], total = %d; want [r2 r3], 2", rows[0].ID, rows[1].ID, total)
	}

	rows, total, err = repo.Query(RequestQuery{Page: 1, PageSize: 50, Provider: "provider-b", Scenario: "complex"})
	if err != nil {
		t.Fatalf("exact filters: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "r3" {
		t.Fatalf("exact-filter rows = %+v, total = %d; want only r3", rows, total)
	}
}

func timePtr(value time.Time) *time.Time { return &value }
