package storage

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestCatalogTierBackfillMatchesInsert(t *testing.T) {
	db := newCostTestDB(t)
	for _, provider := range []string{"openrouter", "opencode-go"} {
		_, err := db.DB().Exec(`INSERT INTO models (id, provider, name, cost_input_per_m, cost_output_per_m, cost_tiers) VALUES (?, ?, ?, ?, ?, ?)`,
			provider+"/synthetic-tier", provider, "synthetic-tier", 1, 2, `[{"min_prompt_tokens":100,"input":3,"output":4}]`)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		id, provider       string
		input, cacheCreate int
		want               float64
	}{
		{"at-threshold", "openrouter", 100, 0, 0.00012},
		{"above-threshold", "openrouter", 101, 0, 0.000343},
		{"cache-crosses-threshold", "opencode-go", 1, 100, 0.000343},
	} {
		t.Run(tc.id, func(t *testing.T) {
			id := tc.id
			record := history.RequestRecord{ID: id, Provider: tc.provider, Model: "synthetic-tier", StartTime: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC), InputTokens: tc.input, CacheCreationTokens: tc.cacheCreate, OutputTokens: 10, Success: true}
			if err := NewRequests(db).Insert(record); err != nil {
				t.Fatal(err)
			}
			var inserted float64
			if err := db.DB().QueryRow(`SELECT cost_usd FROM requests WHERE id = ?`, id).Scan(&inserted); err != nil {
				t.Fatal(err)
			}
			if math.Abs(inserted-tc.want) > 1e-12 {
				t.Fatalf("Insert cost=%g want=%g", inserted, tc.want)
			}
			if _, err := db.DB().Exec(`UPDATE requests SET cost_usd = NULL, cost_source = NULL WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}
			updated, err := db.BackfillRequestCosts(context.Background())
			if err != nil || updated != 1 {
				t.Fatalf("backfill updated=%d err=%v", updated, err)
			}
			var backfilled float64
			if err := db.DB().QueryRow(`SELECT cost_usd FROM requests WHERE id = ?`, id).Scan(&backfilled); err != nil {
				t.Fatal(err)
			}
			if math.Abs(backfilled-inserted) > 1e-12 {
				t.Errorf("backfill=%g insert=%g", backfilled, inserted)
			}
			updated, err = db.BackfillRequestCosts(context.Background())
			if err != nil || updated != 0 {
				t.Fatalf("repeat backfill updated=%d err=%v", updated, err)
			}
		})
	}
}

func TestCatalogTierBackfillRollsBackWholeBatch(t *testing.T) {
	db := newCostTestDB(t)
	_, err := db.DB().Exec(`INSERT INTO models (id, provider, name, cost_input_per_m, cost_output_per_m, cost_tiers)
		VALUES ('openrouter/synthetic-tier', 'openrouter', 'synthetic-tier', 1, 2, '[{"min_prompt_tokens":100,"input":3,"output":4}]')`)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"first", "second"} {
		if err := NewRequests(db).Insert(history.RequestRecord{ID: id, Provider: "openrouter", Model: "synthetic-tier",
			StartTime: time.Now(), InputTokens: 101, OutputTokens: 10, Success: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.DB().Exec(`UPDATE requests SET cost_usd = NULL, cost_source = NULL;
		CREATE TRIGGER fail_second_cost BEFORE UPDATE OF cost_usd ON requests
		WHEN NEW.cost_usd IS NOT NULL AND EXISTS (SELECT 1 FROM requests WHERE cost_usd IS NOT NULL)
		BEGIN SELECT RAISE(ABORT, 'synthetic second cost failure'); END`); err != nil {
		t.Fatal(err)
	}
	if updated, err := db.BackfillRequestCosts(context.Background()); err == nil || updated != 0 {
		t.Fatalf("expected batch failure, updated=%d err=%v", updated, err)
	}
	var remaining int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM requests WHERE cost_usd IS NULL AND cost_source IS NULL`).Scan(&remaining); err != nil || remaining != 2 {
		t.Fatalf("partial batch persisted: remaining=%d err=%v", remaining, err)
	}
	if _, err := db.DB().Exec(`DROP TRIGGER fail_second_cost`); err != nil {
		t.Fatal(err)
	}
	if updated, err := db.BackfillRequestCosts(context.Background()); err != nil || updated != 2 {
		t.Fatalf("batch did not recover: updated=%d err=%v", updated, err)
	}
}
