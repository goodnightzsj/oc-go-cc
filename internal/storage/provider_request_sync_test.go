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
	observedAt := time.Date(2026, 8, 6, 7, 0, 0, 0, time.UTC)
	capturedAt := time.Now().UTC().Add(time.Minute)

	if err := repo.Insert(history.RequestRecord{
		ID: "exact-local", Model: "deepseek-v4-flash", Provider: "platform-a", Scenario: "complex",
		StartTime: observedAt.Add(-1500 * time.Millisecond), Duration: 1800 * time.Millisecond,
		InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30, Success: true,
	}); err != nil {
		t.Fatalf("insert exact request: %v", err)
	}
	if err := repo.Insert(history.RequestRecord{
		ID: "dirty-local", Model: "deepseek-v4-flash", Provider: "platform-a",
		StartTime: observedAt.Add(time.Second), Duration: 500 * time.Millisecond,
		InputTokens: 9999, OutputTokens: 3, Success: true,
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
		{Time: observedAt, Model: "deepseek-v4-flash", Provider: "snapshot-platform", Plan: "lite", InputTokens: 10, OutputTokens: 2, CacheReadTokens: 30, ProviderCostUnits: 1234},
		{Time: observedAt.Add(time.Second), Model: "kimi-k2.6", Provider: "snapshot-platform", Plan: "lite", InputTokens: 20, OutputTokens: 4, CacheWrite5mTokens: 5, ProviderCostUnits: 5678},
	}
	if err := db.ReplaceProviderUsage(context.Background(), capturedAt, providerRows); err != nil {
		t.Fatalf("replace provider usage: %v", err)
	}

	dry, err := db.SyncProviderUsageRequests(context.Background(), false)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.SnapshotRows != 2 || dry.MatchedDetails != 1 || dry.WouldRemove != 1 || dry.WouldInsert != 1 || dry.ProjectedRequests != 3 {
		t.Fatalf("unexpected dry-run report: %+v", dry)
	}
	if count, _ := repo.Count(); count != 3 {
		t.Fatalf("dry run changed request count to %d", count)
	}

	applied, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Removed != 1 || applied.Inserted != 1 || applied.ProjectedRequests != 3 {
		t.Fatalf("unexpected apply report: %+v", applied)
	}
	if math.Abs(applied.ProjectedCostUSD-(1+0.00001234+0.00005678)) > 1e-12 {
		t.Fatalf("projected cost = %.8f", applied.ProjectedCostUSD)
	}

	records, err := repo.Last(10)
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
	if _, ok := byID["dirty-local"]; ok {
		t.Fatal("dirty local row was not removed")
	}
	var imported history.RequestRecord
	for id, rec := range byID {
		if id != "exact-local" && id != "later-live" {
			imported = rec
		}
	}
	if imported.ID == "" || imported.DetailsKnown || imported.Model != "kimi-k2.6" || imported.Provider != "platform-a" || imported.CacheCreationTokens != 5 {
		t.Fatalf("unexpected imported row: %+v", imported)
	}
	if byID["exact-local"].Provider != "platform-a" {
		t.Fatalf("exact request provider changed: %+v", byID["exact-local"])
	}
	analytics := NewAnalytics(db)
	analytics.SetBaseline(observedAt.Add(30 * time.Minute))
	summary, err := analytics.GetTokenSummary(30)
	if err != nil {
		t.Fatalf("analytics after sync: %v", err)
	}
	if summary.TotalRequests != 3 {
		t.Fatalf("analytics requests = %d, want both corrected rows plus later live row", summary.TotalRequests)
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
