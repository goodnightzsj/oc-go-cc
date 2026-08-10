package storage

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestProviderCostReconciliationClassifiesWithoutGuessing(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	base := time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)

	for _, rec := range []history.RequestRecord{
		{ID: "exact", Model: "opencode-go/deepseek-v4-flash", StartTime: base.Add(-2038 * time.Millisecond), Duration: 2705 * time.Millisecond, CacheCreationTokens: 10, OutputTokens: 2, CacheReadTokens: 30, Success: true},
		{ID: "ambiguous-a", Model: "deepseek-v4-flash", StartTime: base.Add(-time.Second), Duration: 2200 * time.Millisecond, CacheCreationTokens: 20, OutputTokens: 3, Success: true},
		{ID: "ambiguous-b", Model: "deepseek-v4-flash", StartTime: base.Add(200 * time.Millisecond), Duration: time.Second, CacheCreationTokens: 20, OutputTokens: 3, Success: true},
		{ID: "conflict", Model: "deepseek-v4-flash", StartTime: base.Add(time.Second), Duration: 1200 * time.Millisecond, CacheCreationTokens: 40, OutputTokens: 4, Success: true},
	} {
		if err := repo.Insert(rec); err != nil {
			t.Fatalf("insert %s: %v", rec.ID, err)
		}
	}

	providerRows := []ProviderCostRecord{
		{Time: base, Model: "deepseek-v4-flash", InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30, ProviderCostUnits: 1234},
		{Time: base.Add(time.Second), Model: "deepseek-v4-flash", InputTokens: 20, OutputTokens: 3, ProviderCostUnits: 2345},
		{Time: base.Add(2 * time.Second), Model: "deepseek-v4-flash", InputTokens: 41, OutputTokens: 4, ProviderCostUnits: 3456},
		{Time: base.Add(3 * time.Second), Model: "deepseek-v4-flash", InputTokens: 50, OutputTokens: 5, ProviderCostUnits: 4567},
	}

	report, err := db.ReconcileProviderCosts(context.Background(), providerRows, false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if report.Exact != 1 || report.Ambiguous != 1 || report.Conflicting != 1 || report.Missing != 1 || report.WouldUpdate != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	report, err = db.ReconcileProviderCosts(context.Background(), providerRows, true)
	if !errors.Is(err, ErrAmbiguousProviderCosts) {
		t.Fatalf("apply error = %v, want ErrAmbiguousProviderCosts", err)
	}
	if report.Updated != 0 {
		t.Fatalf("updated = %d, want 0 when any match is ambiguous", report.Updated)
	}
	assertRequestCostSource(t, db, "exact", CostSourceEstimated)
}

func TestProviderCostReconciliationAppliesExactMatches(t *testing.T) {
	db := newCostTestDB(t)
	repo := NewRequests(db)
	completedAt := time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)
	startedAt := completedAt.Add(-1662 * time.Millisecond)
	if err := repo.Insert(history.RequestRecord{
		ID: "request-1", Model: "deepseek-v4-flash", StartTime: startedAt, Duration: 2705 * time.Millisecond,
		CacheCreationTokens: 10, OutputTokens: 2, CacheReadTokens: 30, Success: true,
	}); err != nil {
		t.Fatalf("insert request: %v", err)
	}

	row := ProviderCostRecord{
		Time: completedAt, Model: "deepseek-v4-flash",
		InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30,
		ProviderCostUnits: 1234,
	}
	report, err := db.ReconcileProviderCosts(context.Background(), []ProviderCostRecord{row}, true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if report.Exact != 1 || report.WouldUpdate != 1 || report.Updated != 1 {
		t.Fatalf("unexpected apply report: %+v", report)
	}

	var cost float64
	var source string
	if err := db.DB().QueryRow(`SELECT cost_usd, cost_source FROM requests WHERE id = 'request-1'`).Scan(&cost, &source); err != nil {
		t.Fatalf("read applied cost: %v", err)
	}
	if math.Abs(cost-0.00001234) > 1e-12 || source != CostSourceProvider {
		t.Fatalf("applied cost/source = %.8f/%q, want 0.00001234/%q", cost, source, CostSourceProvider)
	}

	report, err = db.ReconcileProviderCosts(context.Background(), []ProviderCostRecord{row}, true)
	if err != nil {
		t.Fatalf("idempotent apply: %v", err)
	}
	if report.Exact != 1 || report.WouldUpdate != 0 || report.Updated != 0 {
		t.Fatalf("unexpected idempotent report: %+v", report)
	}
	assertRequestCostSource(t, db, "request-1", CostSourceProvider)
}

func assertRequestCostSource(t *testing.T, db *Database, id, want string) {
	t.Helper()
	var got string
	if err := db.DB().QueryRow(`SELECT cost_source FROM requests WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatalf("read cost source for %s: %v", id, err)
	}
	if got != want {
		t.Fatalf("cost source for %s = %q, want %q", id, got, want)
	}
}
