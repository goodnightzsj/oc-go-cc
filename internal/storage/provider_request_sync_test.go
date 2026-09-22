package storage

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestSyncProviderUsageRequestsCorrectsHistoryAndIsIdempotent(t *testing.T) {
	db, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "sync.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := NewRequests(db)
	observedAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	capturedAt := time.Now().UTC().Add(time.Minute)

	if err := repo.Insert(history.RequestRecord{
		ID: "exact-local", Model: "deepseek-v4-flash", Provider: "opencode-go", Scenario: "complex",
		StartTime: observedAt.Add(-1500 * time.Millisecond), Duration: 1800 * time.Millisecond,
		InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30, Success: true,
	}); err != nil {
		t.Fatalf("insert exact request: %v", err)
	}
	if err := repo.Insert(history.RequestRecord{
		ID: "dirty-local", Model: "deepseek-v4-flash", Provider: "opencode-go",
		StartTime: observedAt.Add(time.Second), Duration: 500 * time.Millisecond,
		InputTokens: 9999, OutputTokens: 3, Success: true,
		CostKnown: true, CostUSD: 0.25,
	}); err != nil {
		t.Fatalf("insert dirty request: %v", err)
	}
	if err := repo.Insert(history.RequestRecord{
		ID: "later-live", Model: "future-model", Provider: "platform-b",
		StartTime: observedAt.Add(time.Hour), Duration: time.Second, Success: true,
		CostKnown: true, CostUSD: 1, CostSource: CostSourceProvider,
	}); err != nil {
		t.Fatalf("insert later request: %v", err)
	}

	providerRows := []ProviderCostRecord{
		{Time: observedAt, Model: "deepseek-v4-flash", Provider: "inf-go.oa-compat", Plan: "lite", InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30, ProviderCostUnits: 1234},
		{Time: observedAt.Add(time.Second), Model: "kimi-k2.6", Provider: "inf-go.oa-compat", Plan: "lite", InputTokens: 20, OutputTokens: 4, CacheWrite5mTokens: 5, ProviderCostUnits: 5678},
	}
	if err := db.ReplaceProviderUsage(context.Background(), capturedAt, providerRows); err != nil {
		t.Fatalf("replace provider usage: %v", err)
	}

	dry, err := db.SyncProviderUsageRequests(context.Background(), false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.TargetProvider != "opencode-go" || dry.SnapshotRows != 2 || dry.MatchedDetails != 1 || dry.WouldRemove != 0 || dry.PreservedUnmatched != 1 || dry.WouldInsert != 1 || dry.ProjectedRequests != 4 {
		t.Fatalf("unexpected dry-run report: %+v", dry)
	}
	if _, count, _ := repo.Query(RequestQuery{}); count != 3 {
		t.Fatalf("dry run changed request count to %d", count)
	}

	applied, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Removed != 0 || applied.PreservedUnmatched != 1 || applied.Inserted != 1 || applied.ProjectedRequests != 4 {
		t.Fatalf("unexpected apply report: %+v", applied)
	}
	if math.Abs(applied.ProjectedCostUSD-(1.25+0.00001234+0.00005678)) > 1e-12 {
		t.Fatalf("projected cost = %.8f", applied.ProjectedCostUSD)
	}

	records, _, err := repo.Query(RequestQuery{PageSize: 10})
	if err != nil {
		t.Fatalf("read requests: %v", err)
	}
	byID := map[string]history.RequestRecord{}
	for _, rec := range records {
		byID[rec.ID] = rec
	}
	if !byID["exact-local"].DetailsKnown || byID["exact-local"].CostSource != CostSourceProvider {
		t.Fatalf("exact local row was not preserved: %+v", byID["exact-local"])
	}
	if rec, ok := byID["dirty-local"]; !ok || rec.InputTokens != 9999 || rec.CostUSD != 0.25 {
		t.Fatal("unmatched local row was removed or changed")
	}
	var imported history.RequestRecord
	for _, rec := range byID {
		if !rec.DetailsKnown {
			imported = rec
		}
	}
	if imported.ID == "" || imported.DetailsKnown || imported.Model != "kimi-k2.6" || imported.Provider != "opencode-go" || imported.Scenario != "override" || imported.CacheCreationTokens != 5 {
		t.Fatalf("unexpected imported row: %+v", imported)
	}
	if byID["exact-local"].Provider != "opencode-go" {
		t.Fatalf("exact request provider changed: %+v", byID["exact-local"])
	}
	analytics := &Analytics{db: db, baseline: observedAt.Add(30 * time.Minute)}
	summary, err := analytics.TokenSummary(analytics.Window(30))
	if err != nil {
		t.Fatalf("analytics after sync: %v", err)
	}
	if summary.TotalRequests != 3 {
		t.Fatalf("analytics requests = %d, want both corrected rows plus later live row", summary.TotalRequests)
	}
	if _, err := db.DB().Exec(`UPDATE requests SET scenario = '' WHERE id = ?`, imported.ID); err != nil {
		t.Fatalf("clear imported scenario: %v", err)
	}
	scenarioDryRun, err := db.SyncProviderUsageRequests(context.Background(), false)
	if err != nil {
		t.Fatalf("scenario dry run: %v", err)
	}
	if scenarioDryRun.WouldUpdate != 1 {
		t.Fatalf("scenario dry run updates = %d, want 1", scenarioDryRun.WouldUpdate)
	}
	if _, err := db.SyncProviderUsageRequests(context.Background(), true); err != nil {
		t.Fatalf("repair imported scenario: %v", err)
	}
	var repairedScenario string
	if err := db.DB().QueryRow(`SELECT scenario FROM requests WHERE id = ?`, imported.ID).Scan(&repairedScenario); err != nil {
		t.Fatalf("read repaired scenario: %v", err)
	}
	if repairedScenario != "override" {
		t.Fatalf("repaired scenario = %q, want override", repairedScenario)
	}

	idempotent, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err != nil {
		t.Fatalf("idempotent apply: %v", err)
	}
	if idempotent.WouldInsert != 0 || idempotent.WouldRemove != 0 || idempotent.WouldUpdate != 0 || idempotent.Inserted != 0 || idempotent.Removed != 0 {
		t.Fatalf("second apply is not idempotent: %+v", idempotent)
	}
}

func TestSyncProviderUsageRequestsRequiresSnapshot(t *testing.T) {
	db := newCostTestDB(t)
	if _, err := db.SyncProviderUsageRequests(context.Background(), false); err == nil {
		t.Fatal("sync without provider snapshot succeeded")
	}
}

// TestSyncInsertedRowsCarryThePeakMultiplier. The provider-sync path writes its
// own INSERT column list rather than going through peakMultiplierForRecord, the
// helper every other writer uses, and that list omitted peak_multiplier - so a
// row imported from OpenCode Go's own billing snapshot was stored at the
// column's DEFAULT of 1.
//
// That is the platform's peak-priced traffic, so the effect was a request billed
// at twice the off-peak rate being displayed and totalled at the off-peak rate.
// Nothing reported it, and the startup backfill silently corrected it on the next
// service start, so the same row showed one number before a restart and another
// after.
//
// 2026-09-07 is a Monday and 02:00 UTC sits inside the 01-04 window, so the
// expected multiplier is 2 and a DEFAULT of 1 cannot pass by coincidence.
func TestSyncInsertedRowsCarryThePeakMultiplier(t *testing.T) {
	db, err := Open(Config{DatabasePath: filepath.Join(t.TempDir(), "peak.db")})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = db.Close() }()

	observedAt := time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC)
	if got := history.ProviderPeakMultiplier("opencode-go", "deepseek-v4-flash", observedAt); got != 2 {
		t.Fatalf("fixture is not in the peak window: multiplier %v", got)
	}
	capturedAt := observedAt.Add(time.Minute)

	// No local rows, so the snapshot row below has no candidate to match and is
	// imported - which is the branch that writes the column.
	providerRows := []ProviderCostRecord{
		{Time: observedAt, Model: "deepseek-v4-flash", Provider: "inf-go.oa-compat", Plan: "lite",
			InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30, ProviderCostUnits: 1234},
	}
	if err := db.ReplaceProviderUsage(context.Background(), capturedAt, providerRows); err != nil {
		t.Fatalf("replace provider usage: %v", err)
	}
	report, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if report.Inserted != 1 {
		t.Fatalf("inserted = %d, want 1: %+v", report.Inserted, report)
	}

	var peak float64
	if err := db.DB().QueryRow(`SELECT peak_multiplier FROM requests WHERE model = ?`, "deepseek-v4-flash").Scan(&peak); err != nil {
		t.Fatalf("read peak_multiplier: %v", err)
	}
	if peak != 2 {
		t.Errorf("imported peak-window row has peak_multiplier = %v, want 2", peak)
	}

	// And the row must not depend on the backfill to be right: a second pass
	// finds nothing to correct, which is only true if the insert already set it.
	if n, err := db.BackfillPeakMultipliers(context.Background()); err != nil {
		t.Fatalf("backfill: %v", err)
	} else if n != 0 {
		t.Errorf("backfill corrected %d rows, so the insert did not carry the multiplier", n)
	}
}
