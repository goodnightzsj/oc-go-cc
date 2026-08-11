package storage

import (
	"context"
	"database/sql"
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
		{ID: "r3", Model: "model-b", Provider: "provider-b", Scenario: "complex", StartTime: base.Add(3 * time.Hour), InputTokens: 100, OutputTokens: 10, CostUSD: 0.25, CostKnown: true, CostSource: CostSourceProvider, Streaming: true, Success: true},
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

	rows, total, err = repo.Query(RequestQuery{Page: 1, PageSize: 50, CostSource: CostSourceProvider})
	if err != nil {
		t.Fatalf("cost-source filter: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "r3" {
		t.Fatalf("provider-cost rows = %+v, total = %d; want only r3", rows, total)
	}
	summary, err := repo.Summary(RequestQuery{Provider: "provider-b", Scenario: "complex"})
	if err != nil {
		t.Fatalf("filtered summary: %v", err)
	}
	if summary.TotalRequests != 1 || summary.TotalTokens != 110 || summary.InputTokens != 100 ||
		summary.OutputTokens != 10 || summary.CacheReadTokens != 0 || summary.CacheCreationTokens != 0 || summary.CostUSD != 0.25 ||
		len(summary.Models) != 1 || summary.Models[0].Name != "model-b" || len(summary.Trend) != 1 {
		t.Fatalf("filtered summary = %+v; want r3 aggregates", summary)
	}

	rows, total, err = repo.Query(RequestQuery{Page: 1, PageSize: 50, SortBy: "cost_usd", SortOrder: "desc"})
	if err != nil {
		t.Fatalf("cost sort: %v", err)
	}
	if total != 4 || len(rows) != 4 || rows[0].CostUSD < rows[1].CostUSD {
		t.Fatalf("cost-sorted rows = %+v, total = %d; want descending costs", rows, total)
	}
}

func timePtr(value time.Time) *time.Time { return &value }

func TestBackfillRequestCostsMigratesExistingRows(t *testing.T) {
	db := newCostTestDB(t)
	cutoff := time.Now().Add(-time.Hour)
	db.analyticsBaseline = cutoff
	repo := NewRequests(db)
	for _, rec := range []history.RequestRecord{
		{ID: "old", StartTime: cutoff.Add(-time.Hour)},
		{ID: "priced", StartTime: cutoff.Add(time.Minute)},
	} {
		rec.Model = "deepseek-v4-flash"
		rec.Provider = "opencode-go"
		rec.CacheCreationTokens = 458090
		rec.OutputTokens = 114821
		rec.CacheReadTokens = 123418752
		rec.Success = true
		if err := repo.Insert(rec); err != nil {
			t.Fatalf("insert %s: %v", rec.ID, err)
		}
	}
	if _, err := db.DB().Exec(`UPDATE requests SET cost_usd = NULL`); err != nil {
		t.Fatalf("clear cost_usd: %v", err)
	}

	updated, err := db.BackfillRequestCosts(context.Background())
	if err != nil {
		t.Fatalf("BackfillRequestCosts: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated = %d, want 1", updated)
	}
	var oldCost, newCost sql.NullFloat64
	if err := db.DB().QueryRow(`SELECT cost_usd FROM requests WHERE id = 'old'`).Scan(&oldCost); err != nil {
		t.Fatalf("query old cost: %v", err)
	}
	if err := db.DB().QueryRow(`SELECT cost_usd FROM requests WHERE id = 'priced'`).Scan(&newCost); err != nil {
		t.Fatalf("query new cost: %v", err)
	}
	if oldCost.Valid || !newCost.Valid || newCost.Float64 <= 0 {
		t.Fatalf("costs = old:%v new:%v; want old NULL and new positive", oldCost, newCost)
	}
	rows, _, err := repo.Query(RequestQuery{Page: 1, PageSize: 1, SortBy: "cost_usd"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 || rows[0].CostUSD <= 0 {
		t.Fatalf("backfilled row = %+v, want positive cost", rows)
	}
}
