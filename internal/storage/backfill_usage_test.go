package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestBackfillPreservesUsageKnowledgeAcrossReopen(t *testing.T) {
	cfg := Config{DatabasePath: filepath.Join(t.TempDir(), "usage.db")}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, id := range []string{"unobserved", "observed-zero", "trusted"} {
		insertCostRecord(t, db, history.RequestRecord{
			ID: id, Provider: "workbuddy", Model: "global:deepseek-v4.1-flash",
			StartTime: time.Now(), UsageUnobserved: id == "unobserved",
		})
	}
	// Imported usage can be trusted even when local request details are absent.
	if _, err := db.DB().Exec(`UPDATE requests SET details_known = 0, usage_trusted = 1, input_tokens = 1000 WHERE id = 'trusted'`); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"explicit", "startup"} {
		t.Run(phase, func(t *testing.T) {
			if _, err := db.DB().Exec(`UPDATE requests SET cost_usd = NULL, cost_source = NULL`); err != nil {
				t.Fatal(err)
			}
			if phase == "explicit" {
				if n, err := db.BackfillRequestCosts(context.Background()); err != nil || n != 2 {
					t.Errorf("backfill = %d, %v; want 2, nil", n, err)
				}
			} else {
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				reopened, err := Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				db = reopened
			}
			for _, id := range []string{"unobserved", "observed-zero", "trusted"} {
				var cost sql.NullFloat64
				if err := db.DB().QueryRow(`SELECT cost_usd FROM requests WHERE id = ?`, id).Scan(&cost); err != nil {
					t.Fatal(err)
				}
				if cost.Valid != (id != "unobserved") || (id == "observed-zero" && cost.Float64 != 0) || (id == "trusted" && cost.Float64 <= 0) {
					t.Errorf("%s cost = %+v", id, cost)
				}
			}
		})
	}
}
