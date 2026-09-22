package storage

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// The history table's "Tokens" column shows the four-part sum (input + output +
// cache read + cache creation) and sorts by that same figure. It used to sort by
// prompt_tokens, which excludes output, so a row with a small prompt and a large
// answer displayed a large total and sorted among the small ones.
//
// The fixture is built so that inserting, prompt and total orderings each pick a
// different row first. That is the whole point: with the sort key deleted the
// query falls back to the default ordering (julianday(start_time)), so a fixture
// whose time order happens to match its total order would pass with the fix
// absent - a test that cannot fail. Four rows:
//
//	row   prompt = in+cr+cc   total = prompt+out   start_time
//	a     30                  30                   t+4h
//	b      5                  35                   t+2h
//	c    100                 110                   t+1h
//	d     12                  12                   t+3h
//
// Ascending prompt -> b,d,a,c. Ascending total -> d,a,b,c. Ascending time ->
// c,b,d,a. Distinct, so each failure names the ordering that actually ran.
func TestHistoryTokensColumnSortsByTheTotalItDisplays(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	base := time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	for _, rec := range []history.RequestRecord{
		{ID: "a", Model: "model-a", StartTime: base.Add(4 * time.Hour), InputTokens: 10, CacheReadTokens: 20, Success: true},
		{ID: "b", Model: "model-b", StartTime: base.Add(2 * time.Hour), InputTokens: 5, OutputTokens: 30, Success: true},
		{ID: "c", Model: "model-c", StartTime: base.Add(time.Hour), InputTokens: 100, OutputTokens: 10, Success: true},
		{ID: "d", Model: "model-d", StartTime: base.Add(3 * time.Hour), CacheCreationTokens: 12, Success: true},
	} {
		if err := repo.Insert(rec); err != nil {
			t.Fatalf("insert %s: %v", rec.ID, err)
		}
	}
	ordered := func(sortBy string) string {
		t.Helper()
		rows, _, err := repo.Query(RequestQuery{Page: 1, PageSize: 10, SortBy: sortBy, SortOrder: "asc"})
		if err != nil {
			t.Fatalf("sort by %s: %v", sortBy, err)
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		return strings.Join(ids, "")
	}
	if got := ordered("total_tokens"); got != "dabc" {
		t.Errorf("ascending total_tokens = %q, want dabc (totals 12,30,35,110)", got)
	}
	// The old key still exists and still means the prompt figure, so a caller
	// wanting the input-only ordering is not silently handed the total.
	if got := ordered("prompt_tokens"); got != "bdac" {
		t.Errorf("ascending prompt_tokens = %q, want bdac (prompts 5,12,30,100)", got)
	}
	// An unknown key falls to the default time ordering; asserting it keeps the
	// three orderings distinguishable, which is what makes the two above mean
	// something rather than passing on a coincidence.
	if got := ordered("not_a_column"); got != "cbda" {
		t.Errorf("unknown sort key = %q, want the default time order cbda", got)
	}
}

// TestRequestsQueryFiltersAndSortsFullDataset covers the filters and the sort
// keys the history page does not use.
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

func TestRequestsQueryPreservesUnknownOutcomesAndUsage(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	for _, tt := range []struct {
		id           string
		detailsKnown int
		success      any
		wantKnown    bool
		wantSuccess  bool
	}{
		{"success", 1, 1, true, true},
		{"failure", 1, 0, true, false},
		{"legacy-null", 1, nil, false, false},
		{"invalid-positive", 1, 2, false, false},
		{"invalid-negative", 1, -1, false, false},
		{"imported-success", 0, 1, false, true},
		{"imported-failure", 0, 0, false, false},
	} {
		t.Run(tt.id, func(t *testing.T) {
			original := history.RequestRecord{
				ID: tt.id, Model: "shared-model", Provider: "commandcode", Scenario: "default",
				StartTime: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), Duration: 1500 * time.Millisecond,
				InputTokens: 11, OutputTokens: 7, CacheReadTokens: 3, CacheCreationTokens: 5,
				CostUSD: 0.125, CostKnown: true, CostSource: CostSourceProvider,
				Streaming: true, Attempt: 2, ErrorMsg: "retained diagnostic",
			}
			if err := repo.Insert(original); err != nil {
				t.Fatal(err)
			}
			if _, err := db.DB().Exec(`UPDATE requests SET details_known = ?, success = ? WHERE id = ?`, tt.detailsKnown, tt.success, tt.id); err != nil {
				t.Fatal(err)
			}
			rows, count, err := repo.Query(RequestQuery{Search: tt.id})
			if err != nil || count != 1 || len(rows) != 1 {
				t.Fatalf("Query = %+v, %d, %v", rows, count, err)
			}
			got := rows[0]
			if got.DetailsKnown != tt.wantKnown || got.Success != tt.wantSuccess {
				t.Errorf("outcome = known:%v success:%v, want known:%v success:%v", got.DetailsKnown, got.Success, tt.wantKnown, tt.wantSuccess)
			}
			if got.InputTokens != original.InputTokens || got.OutputTokens != original.OutputTokens ||
				got.CacheReadTokens != original.CacheReadTokens || got.CacheCreationTokens != original.CacheCreationTokens ||
				got.DisplayInputTokens() != 19 || got.CostUSD != original.CostUSD || !got.CostKnown || got.CostSource != CostSourceProvider {
				t.Errorf("unknown outcome changed available usage or cost: %+v", got)
			}
			if got.Duration != original.Duration || got.Streaming != original.Streaming || got.Attempt != original.Attempt || got.ErrorMsg != original.ErrorMsg {
				t.Errorf("unknown outcome erased detail values: %+v", got)
			}
		})
	}
	for _, success := range []bool{false, true} {
		rows, count, err := repo.Query(RequestQuery{Success: &success})
		if err != nil || count != 1 || len(rows) != 1 {
			t.Fatalf("filter success=%v: rows=%+v count=%d error=%v", success, rows, count, err)
		}
		if !rows[0].DetailsKnown || rows[0].Success != success {
			t.Errorf("filter success=%v included unknown or mismatched outcome: %+v", success, rows[0])
		}
	}
	summary, err := repo.Summary(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalRequests != 7 || summary.SuccessRows != 2 || summary.SuccessRate != 0.5 || summary.TotalTokens != 7*26 || summary.CostUSD != 7*0.125 {
		t.Errorf("summary must preserve all usage and count only known outcomes: %+v", summary)
	}
}
