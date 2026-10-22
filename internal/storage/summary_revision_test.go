package storage

import (
	"context"
	"github.com/routatic/proxy/internal/history"
	"math"
	"testing"
	"time"
)

func TestSummarySeesReconciledCosts(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	completedAt := time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "review-reconcile", Model: "review-model", Provider: "opencode-go",
		StartTime: completedAt.Add(-time.Second), Duration: time.Second,
		InputTokens: 10, OutputTokens: 2, Success: true,
		CostKnown: true, CostUSD: 1, CostSource: CostSourceEstimated,
	})
	before, err := repo.Summary(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if before.CostUSD != 1 {
		t.Fatalf("initial cost=%v", before.CostUSD)
	}
	providerBefore, err := repo.Summary(RequestQuery{CostSource: CostSourceProvider})
	if err != nil {
		t.Fatal(err)
	}
	if providerBefore.TotalRequests != 0 {
		t.Fatalf("initial provider rows=%d", providerBefore.TotalRequests)
	}
	// A separate Database handle models the supported costs reconcile CLI
	// updating the file while the dashboard process retains its cache.
	writer, err := Open(Config{DatabasePath: db.path, WALEnabled: false})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	report, err := writer.ReconcileProviderCosts(context.Background(), "opencode-go", []ProviderCostRecord{{
		Time: completedAt, Model: "review-model", InputTokens: 10, OutputTokens: 2, ProviderCostUSD: 2,
	}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated != 1 {
		t.Fatalf("report=%+v", report)
	}
	recs, _, err := repo.Query(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || math.Abs(recs[0].CostUSD-2) > 1e-12 {
		t.Fatalf("current rows=%+v", recs)
	}
	after, err := repo.Summary(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if after.CostUSD != 2 {
		t.Errorf("summary cost=%v but persisted row cost=%v", after.CostUSD, recs[0].CostUSD)
	}
	providerAfter, err := repo.Summary(RequestQuery{CostSource: CostSourceProvider})
	if err != nil {
		t.Fatal(err)
	}
	if providerAfter.TotalRequests != 1 {
		t.Errorf("provider summary rows=%d, want 1", providerAfter.TotalRequests)
	}
}

func TestHistoryRevisionTracksAllCommittedWrites(t *testing.T) {
	db := newCostTestDB(t)
	ctx := context.Background()
	repo := NewRequests(db)
	writer, err := Open(Config{DatabasePath: db.path})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	revision := func() string {
		t.Helper()
		v, err := repo.historyRevision(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	prev := revision()
	for _, sql := range []string{
		"INSERT INTO requests (id,model,start_time,cost_usd) VALUES ('revision','m','2026-10-23',1)",
		"UPDATE requests SET cost_usd=2 WHERE id='revision'",
		"DELETE FROM requests WHERE id='revision'",
		"INSERT INTO requests (id,model,start_time,cost_usd) VALUES ('revision','m','2026-10-23',3)",
		"INSERT OR REPLACE INTO requests (id,model,start_time,cost_usd) VALUES ('revision','m','2026-10-23',4)",
	} {
		if _, err := writer.DB().ExecContext(ctx, sql); err != nil {
			t.Fatal(err)
		}
		next := revision()
		if next == prev {
			t.Fatalf("revision unchanged after %s", sql)
		}
		prev = next
	}
	before, err := repo.Summary(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	hit, err := repo.Summary(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if before != hit {
		t.Fatal("unchanged data missed cache")
	}
	tx, err := writer.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE requests SET cost_usd=9"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if revision() != prev {
		t.Fatal("rolled back transaction invalidated revision")
	}
	after, err := repo.Summary(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if after.CostUSD != 4 {
		t.Fatal("rollback changed summary")
	}
}
