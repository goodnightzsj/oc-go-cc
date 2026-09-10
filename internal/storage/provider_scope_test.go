package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestProviderCostReconciliationScopesPlatform(t *testing.T) {
	db := newCostTestDB(t)
	when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	for _, provider := range []string{"opencode-go", "openrouter", ""} {
		insertCostRecord(t, db, history.RequestRecord{
			ID: "scope-" + provider, Provider: provider, Model: "review-model", StartTime: when,
			InputTokens: 10, OutputTokens: 2, CostKnown: true, CostUSD: 1,
		})
	}
	rows := []ProviderCostRecord{{Time: when, Model: "review-model", Provider: "inf-go.oa-compat", InputTokens: 10, OutputTokens: 2, ProviderCostUSD: 0.5}}
	report, err := db.ReconcileProviderCosts(context.Background(), "opencode_go", rows, true)
	if err != nil || report.TargetProvider != "opencode-go" || report.Exact != 1 || report.Updated != 1 || report.Ambiguous != 0 {
		t.Fatalf("scoped reconciliation: report=%+v error=%v", report, err)
	}
	records, _, err := NewRequests(db).Query(RequestQuery{})
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range records {
		wantCost, wantSource := 1.0, CostSourceEstimated
		if rec.Provider == "opencode-go" {
			wantCost, wantSource = 0.5, CostSourceProvider
		}
		if rec.CostUSD != wantCost || rec.CostSource != wantSource {
			t.Fatalf("unexpected cost for provider %q: %v/%s", rec.Provider, rec.CostUSD, rec.CostSource)
		}
	}
	other, err := db.ReconcileProviderCosts(context.Background(), "openrouter", rows, true)
	if err != nil || other.TargetProvider != "openrouter" || other.Exact != 1 || other.Updated != 1 {
		t.Fatalf("explicit second platform: report=%+v error=%v", other, err)
	}
	assertRequestCostSource(t, db, "scope-", CostSourceEstimated)
	for _, invalid := range []string{"", " ", "unsupported"} {
		if _, err := db.ReconcileProviderCosts(context.Background(), invalid, rows, true); err == nil {
			t.Errorf("target %q was accepted", invalid)
		}
	}
}

func TestSyncProviderUsageRequestsMatchesLegacyImportedID(t *testing.T) {
	db := newCostTestDB(t)
	when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	insertCostRecord(t, db, history.RequestRecord{
		ID: "legacy-id", Provider: "opencode-go", Model: "review-model", Scenario: "override", StartTime: when,
		InputTokens: 10, CostKnown: true, CostUSD: 0.5, CostSource: CostSourceProvider,
	})
	if _, err := db.DB().Exec(`UPDATE requests SET details_known = 0, usage_trusted = 1 WHERE id = 'legacy-id'`); err != nil {
		t.Fatal(err)
	}
	rows := []ProviderCostRecord{{Time: when, Model: "review-model", Provider: "inf-go.oa-compat", InputTokens: 10, ProviderCostUSD: 0.5}}
	if err := db.ReplaceProviderUsage(context.Background(), time.Now().UTC().Add(time.Minute), rows); err != nil {
		t.Fatal(err)
	}
	report, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err != nil || report.ExistingImported != 1 || report.Inserted != 0 || report.Updated != 0 || report.Removed != 0 {
		t.Fatalf("legacy import was not retained: report=%+v error=%v", report, err)
	}
	assertRequestCostSource(t, db, "legacy-id", CostSourceProvider)
}

func TestSyncProviderUsageRequestsSkipsTokenAndTimeConflicts(t *testing.T) {
	for _, conflict := range []string{"token_conflict", "completion_time_conflict"} {
		t.Run(conflict, func(t *testing.T) {
			db := newCostTestDB(t)
			when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
			startedAt := when
			input := int64(11)
			rows := []ProviderCostRecord{{Time: when, Model: "review-model", InputTokens: input, ProviderCostUSD: 0.5}}
			if conflict == "completion_time_conflict" {
				startedAt = when.Add(2 * time.Second)
				rows[0].InputTokens = 10
				rows = append(rows, ProviderCostRecord{Time: when.Add(3 * time.Second), Model: "review-boundary", InputTokens: 20})
			}
			insertCostRecord(t, db, history.RequestRecord{ID: "local", Provider: "opencode-go", Model: "review-model", StartTime: startedAt, InputTokens: 10, CostKnown: true, CostUSD: 1})
			if err := db.ReplaceProviderUsage(context.Background(), time.Now().UTC().Add(time.Minute), rows); err != nil {
				t.Fatal(err)
			}
			report, err := db.SyncProviderUsageRequests(context.Background(), true)
			if err != nil || report.Conflicting != 1 || report.Updated != 0 || len(report.IssueExamples) != 1 || report.IssueExamples[0].Kind != conflict {
				t.Fatalf("conflict report=%+v error=%v", report, err)
			}
			matched, count, err := NewRequests(db).Query(RequestQuery{Model: "review-model"})
			if err != nil || count != 1 || matched[0].CostUSD != 1 || matched[0].CostSource != CostSourceEstimated {
				t.Fatalf("conflicting request overwritten or duplicated: rows=%+v count=%d error=%v", matched, count, err)
			}
		})
	}
}

func TestSyncProviderUsageRequestsPreservesOtherPlatformsAndPartialHistory(t *testing.T) {
	db := newCostTestDB(t)
	when := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	for _, provider := range []string{"opencode-go", "openrouter", ""} {
		insertCostRecord(t, db, history.RequestRecord{
			ID: "scope-" + provider, Provider: provider, Model: "review-model", StartTime: when,
			InputTokens: 10, OutputTokens: 2, CostKnown: true, CostUSD: 1,
		})
	}
	for i, id := range []string{"unmatched-local", "legacy-import"} {
		insertCostRecord(t, db, history.RequestRecord{
			ID: id, Provider: "opencode-go", Model: id, StartTime: when.Add(time.Duration(i+1) * time.Second),
			InputTokens: 30, CostKnown: true, CostUSD: 1, CostSource: CostSourceProvider,
		})
	}
	if _, err := db.DB().Exec(`UPDATE requests SET details_known = 0, usage_trusted = 1 WHERE id = 'legacy-import'`); err != nil {
		t.Fatal(err)
	}
	rows := []ProviderCostRecord{
		{Time: when, Model: "review-model", Provider: "inf-go.oa-compat", InputTokens: 10, OutputTokens: 2, ProviderCostUSD: 0.5},
		{Time: when.Add(10 * time.Second), Model: "review-missing", Provider: "unrelated-vendor-label", InputTokens: 20, ProviderCostUSD: 0.25},
	}
	if err := db.ReplaceProviderUsage(context.Background(), time.Now().UTC().Add(time.Minute), rows); err != nil {
		t.Fatal(err)
	}
	report, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err != nil || report.TargetProvider != "opencode-go" || report.Removed != 0 || report.PreservedUnmatched != 2 || report.Updated != 1 || report.Inserted != 1 {
		t.Fatalf("partial snapshot sync: report=%+v error=%v", report, err)
	}
	records, count, err := NewRequests(db).Query(RequestQuery{})
	if err != nil || count != 6 {
		t.Fatalf("history count=%d error=%v", count, err)
	}
	for _, rec := range records {
		switch rec.ID {
		case "scope-opencode-go":
			if rec.CostUSD != 0.5 || rec.CostSource != CostSourceProvider {
				t.Fatalf("target request not reconciled: %+v", rec)
			}
		case "scope-openrouter", "scope-", "unmatched-local", "legacy-import":
			if rec.CostUSD != 1 {
				t.Fatalf("unmatched request changed: %+v", rec)
			}
		default:
			if rec.Provider != "opencode-go" || rec.Model != "review-missing" || rec.DetailsKnown {
				t.Fatalf("unexpected import: %+v", rec)
			}
		}
	}
	again, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err != nil || again.Updated != 0 || again.Inserted != 0 || again.Removed != 0 || again.ExistingImported != 1 {
		t.Fatalf("idempotent partial snapshot: report=%+v error=%v", again, err)
	}
}

func TestSyncProviderUsageRequestsRejectsAmbiguousMatchesWithoutWrites(t *testing.T) {
	for _, duplicatedSide := range []string{"local", "snapshot"} {
		t.Run(duplicatedSide, func(t *testing.T) {
			db := newCostTestDB(t)
			when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
			insertCostRecord(t, db, history.RequestRecord{ID: "exact", Provider: "opencode-go", Model: "review-exact", StartTime: when, InputTokens: 10, CostKnown: true, CostUSD: 1})
			rows := []ProviderCostRecord{
				{Time: when, Model: "review-exact", InputTokens: 10, ProviderCostUSD: 0.5},
				{Time: when.Add(time.Second), Model: "review-ambiguous", InputTokens: 20, ProviderCostUSD: 0.25},
			}
			if duplicatedSide == "local" {
				for _, id := range []string{"ambiguous-a", "ambiguous-b"} {
					insertCostRecord(t, db, history.RequestRecord{ID: id, Provider: "opencode-go", Model: "review-ambiguous", StartTime: when.Add(time.Second), InputTokens: 20, CostKnown: true, CostUSD: 1})
				}
			} else {
				rows = append(rows, rows[1])
			}
			if err := db.ReplaceProviderUsage(context.Background(), time.Now().UTC().Add(time.Minute), rows); err != nil {
				t.Fatal(err)
			}
			dry, err := db.SyncProviderUsageRequests(context.Background(), false)
			if err != nil || dry.Ambiguous == 0 || dry.WouldInsert != 0 || len(dry.IssueExamples) == 0 {
				t.Fatalf("ambiguous dry run: report=%+v error=%v", dry, err)
			}
			report, err := db.SyncProviderUsageRequests(context.Background(), true)
			if !errors.Is(err, ErrAmbiguousProviderCosts) || report.Updated != 0 || report.Inserted != 0 || report.Removed != 0 {
				t.Fatalf("ambiguous apply: report=%+v error=%v", report, err)
			}
			assertRequestCostSource(t, db, "exact", CostSourceEstimated)
		})
	}
}

func TestProviderSyncAndReconciliationPreserveConflictingOfficialCost(t *testing.T) {
	for _, imported := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "legacy-import"}[imported], func(t *testing.T) {
			db := newCostTestDB(t)
			when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
			insertCostRecord(t, db, history.RequestRecord{ID: "official", Provider: "opencode-go", Model: "review-model", StartTime: when, InputTokens: 10, CostKnown: true, CostUSD: 2, CostSource: CostSourceProvider})
			if imported {
				if _, err := db.DB().Exec(`UPDATE requests SET details_known = 0 WHERE id = 'official'`); err != nil {
					t.Fatal(err)
				}
			}
			rows := []ProviderCostRecord{{Time: when, Model: "review-model", Provider: "inf-go.oa-compat", InputTokens: 10, ProviderCostUSD: 1}}
			if err := db.ReplaceProviderUsage(context.Background(), time.Now().UTC().Add(time.Minute), rows); err != nil {
				t.Fatal(err)
			}
			synced, err := db.SyncProviderUsageRequests(context.Background(), true)
			if err != nil || synced.Conflicting != 1 || synced.Updated != 0 || synced.Inserted != 0 || len(synced.IssueExamples) != 1 {
				t.Fatalf("conflicting sync: report=%+v error=%v", synced, err)
			}
			reconciled, err := db.ReconcileProviderCosts(context.Background(), "opencode-go", rows, true)
			if err != nil || reconciled.Conflicting != 1 || reconciled.Updated != 0 {
				t.Fatalf("conflicting reconciliation: report=%+v error=%v", reconciled, err)
			}
			records, count, err := NewRequests(db).Query(RequestQuery{})
			if err != nil || count != 1 || records[0].CostUSD != 2 {
				t.Fatalf("official cost overwritten or duplicated: records=%+v count=%d error=%v", records, count, err)
			}
		})
	}
}

func TestProviderCostReconciliationRollbackReportsNoUpdates(t *testing.T) {
	db := newCostTestDB(t)
	when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	var rows []ProviderCostRecord
	for _, id := range []string{"a", "b"} {
		insertCostRecord(t, db, history.RequestRecord{ID: id, Provider: "opencode-go", Model: id, StartTime: when, InputTokens: 10, CostKnown: true, CostUSD: 1})
		rows = append(rows, ProviderCostRecord{Time: when, Model: id, InputTokens: 10, ProviderCostUSD: 0.5})
	}
	if _, err := db.DB().Exec(`CREATE TRIGGER reject_second_cost BEFORE UPDATE OF cost_usd ON requests WHEN OLD.id = 'b' BEGIN SELECT RAISE(ABORT, 'synthetic update failure'); END`); err != nil {
		t.Fatal(err)
	}
	report, err := db.ReconcileProviderCosts(context.Background(), "opencode-go", rows, true)
	if err == nil || report.Updated != 0 {
		t.Fatalf("rollback report=%+v error=%v", report, err)
	}
	assertRequestCostSource(t, db, "a", CostSourceEstimated)
	assertRequestCostSource(t, db, "b", CostSourceEstimated)
}

func TestSyncProviderUsageRequestsRollbackReportsNoWrites(t *testing.T) {
	db := newCostTestDB(t)
	when := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	insertCostRecord(t, db, history.RequestRecord{ID: "exact", Provider: "opencode-go", Model: "review-exact", StartTime: when, InputTokens: 10, CostKnown: true, CostUSD: 1})
	rows := []ProviderCostRecord{
		{Time: when, Model: "review-exact", InputTokens: 10, ProviderCostUSD: 0.5},
		{Time: when.Add(time.Second), Model: "review-import", InputTokens: 20, ProviderCostUSD: 0.25},
	}
	if err := db.ReplaceProviderUsage(context.Background(), time.Now().UTC().Add(time.Minute), rows); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`CREATE TRIGGER reject_import BEFORE INSERT ON requests WHEN NEW.model = 'review-import' BEGIN SELECT RAISE(ABORT, 'synthetic insert failure'); END`); err != nil {
		t.Fatal(err)
	}
	report, err := db.SyncProviderUsageRequests(context.Background(), true)
	if err == nil || report.Updated != 0 || report.Inserted != 0 || report.Removed != 0 {
		t.Fatalf("rollback report=%+v error=%v", report, err)
	}
	assertRequestCostSource(t, db, "exact", CostSourceEstimated)
	_, count, err := NewRequests(db).Query(RequestQuery{})
	if err != nil || count != 1 {
		t.Fatalf("rollback count=%d error=%v", count, err)
	}
}
