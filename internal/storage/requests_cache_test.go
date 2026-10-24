package storage

import (
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// The dashboard polls the history summary every three seconds while the table
// grows by a few rows a minute, so nearly every poll recomputed an identical
// answer from a full scan. The cache is keyed by the filter and validated by a
// cheap fingerprint of the row set, and both halves have to be right: a cache
// that ignores the filter serves one view's numbers under another's heading,
// and one that ignores changes serves yesterday's totals forever.

// summaryCacheFixture is small and distinct per row, so a wrong cache hit shows
// up as a wrong number rather than as a coincidence.
func summaryCacheFixture(t *testing.T, db *Database) {
	t.Helper()
	base := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	for _, rec := range []history.RequestRecord{
		{ID: "c1", Model: "model-a", Provider: "provider-a", Scenario: "default", StartTime: base, InputTokens: 100, Success: true, CostKnown: true, CostUSD: 1},
		{ID: "c2", Model: "model-b", Provider: "provider-b", Scenario: "complex", StartTime: base.Add(time.Hour), InputTokens: 200, Success: true, CostKnown: true, CostUSD: 2},
	} {
		insertCostRecord(t, db, rec)
	}
}

// A repeat of the same query must return the same numbers, and the cache must
// not be the reason they are wrong.
func TestSummaryCacheReturnsTheSameAnswerForTheSameFilter(t *testing.T) {
	db := newCostTestDB(t)
	summaryCacheFixture(t, db)
	repo := NewRequests(db)

	first, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalRequests != 2 || second.TotalRequests != 2 ||
		first.TotalTokens != second.TotalTokens || first.CostUSD != second.CostUSD {
		t.Fatalf("repeat summary differs: %+v vs %+v", first, second)
	}
}

// Two different filters must not share an entry. This is the failure a cache
// keyed too coarsely produces: the second view silently reports the first's
// totals.
func TestSummaryCacheSeparatesFilters(t *testing.T) {
	db := newCostTestDB(t)
	summaryCacheFixture(t, db)
	repo := NewRequests(db)

	all, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if all.TotalRequests != 2 {
		t.Fatalf("unfiltered total = %d, want 2", all.TotalRequests)
	}
	// Same call shape, different provider: must be 1, not the cached 2.
	one, err := repo.Summary(t.Context(), RequestQuery{Provider: "provider-a"})
	if err != nil {
		t.Fatal(err)
	}
	if one.TotalRequests != 1 {
		t.Errorf("provider filter returned %d requests, want 1 (cache leaked across filters)", one.TotalRequests)
	}
	// And the unfiltered answer must still be intact afterwards.
	again, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if again.TotalRequests != 2 {
		t.Errorf("unfiltered total became %d after a filtered query", again.TotalRequests)
	}
	// A search term is part of the key too.
	found, err := repo.Summary(t.Context(), RequestQuery{Search: "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	if found.TotalRequests != 1 {
		t.Errorf("search summary = %d, want 1", found.TotalRequests)
	}
	// As is the channel.
	none, err := repo.Summary(t.Context(), RequestQuery{Channel: "deepseek"})
	if err != nil {
		t.Fatal(err)
	}
	if none.TotalRequests != 0 {
		t.Errorf("channel summary = %d, want 0", none.TotalRequests)
	}
}

// A new row must invalidate the cached answer. Without this the history tab
// would show a total that never advances.
func TestSummaryCacheSeesNewRows(t *testing.T) {
	db := newCostTestDB(t)
	summaryCacheFixture(t, db)
	repo := NewRequests(db)

	before, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if before.TotalRequests != 2 {
		t.Fatalf("initial total = %d, want 2", before.TotalRequests)
	}
	insertCostRecord(t, db, history.RequestRecord{
		ID: "c3", Model: "model-c", Provider: "provider-a",
		StartTime: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC), InputTokens: 300, Success: true,
	})
	after, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if after.TotalRequests != 3 {
		t.Errorf("total after insert = %d, want 3 (cache did not notice the new row)", after.TotalRequests)
	}
	if after.InputTokens != 600 {
		t.Errorf("input tokens after insert = %d, want 600", after.InputTokens)
	}
}

// A delete must invalidate it too, including the case where a delete and an
// insert cancel out in the row count - which is why the fingerprint carries
// MAX(rowid) alongside COUNT(*).
func TestSummaryCacheSeesDeletedAndReplacedRows(t *testing.T) {
	db := newCostTestDB(t)
	summaryCacheFixture(t, db)
	repo := NewRequests(db)

	if _, err := repo.Summary(t.Context(), RequestQuery{}); err != nil {
		t.Fatal(err)
	}
	// Delete one row and add another: the count is unchanged at 2, so a
	// count-only fingerprint would serve the stale answer.
	if _, err := db.DB().Exec(`DELETE FROM requests WHERE id = 'c1'`); err != nil {
		t.Fatal(err)
	}
	insertCostRecord(t, db, history.RequestRecord{
		ID: "c4", Model: "model-d", Provider: "provider-b",
		StartTime: time.Date(2026, 8, 8, 13, 0, 0, 0, time.UTC), InputTokens: 999, Success: true,
	})
	after, err := repo.Summary(t.Context(), RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if after.TotalRequests != 2 {
		t.Fatalf("total = %d, want 2", after.TotalRequests)
	}
	// c1 held 100 input tokens; c4 holds 999. A stale entry would report 300.
	if after.InputTokens != 200+999 {
		t.Errorf("input tokens = %d, want 1199 (delete+insert cancelled out in the count)", after.InputTokens)
	}
}

// The search must stay case-insensitive now that the redundant LOWER() wrapper
// is gone: LIKE does this for ASCII on its own, and this is the assertion that
// says so rather than assuming it.
func TestHistorySearchIsCaseInsensitiveWithoutLower(t *testing.T) {
	db := newCostTestDB(t)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "case-1", Model: "DeepSeek-V4.1-Flash", Provider: "cline-pass",
		StartTime: time.Now().Add(-time.Minute), InputTokens: 10, Success: true,
	})
	insertCostRecord(t, db, history.RequestRecord{
		ID: "case-2", Model: "Kimi-K3", Provider: "cline-pass",
		StartTime: time.Now().Add(-2 * time.Minute), InputTokens: 10, Success: true,
	})
	repo := NewRequests(db)

	for _, term := range []string{"deepseek", "DeepSeek", "DEEPSEEK", "deepSeek"} {
		rows, total, err := repo.Query(RequestQuery{Page: 1, PageSize: 10, Search: term})
		if err != nil {
			t.Fatalf("search %q: %v", term, err)
		}
		if total != 1 || len(rows) != 1 || rows[0].ID != "case-1" {
			t.Errorf("search %q = %d rows (total %d), want only case-1", term, len(rows), total)
		}
	}
	// A term only present in error_msg must still match: the column stays
	// searched even though this instance has never written one.
	insertCostRecord(t, db, history.RequestRecord{
		ID: "case-3", Model: "model-x", Provider: "provider-a",
		StartTime: time.Now().Add(-3 * time.Minute), Success: false, ErrorMsg: "quota exceeded",
	})
	rows, total, err := repo.Query(RequestQuery{Page: 1, PageSize: 10, Search: "quota"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != "case-3" {
		t.Errorf("error search = %d rows (total %d), want only case-3", len(rows), total)
	}
}
