package storage

import (
	"context"
	"testing"
)

// Imported billing history reaches the database with the peak column default,
// so a peak-billed request reads as off-peak everywhere the column is consumed.
// The backfill has to restore exactly those rows and nothing else.
func TestBackfillPeakMultipliersRestoresOnlyUnmarkedPeakRows(t *testing.T) {
	db := newCostTestDB(t)
	// 2026-09-07 is a Monday; 01:30 and 08:00 UTC are peak, 12:00 UTC is not.
	insert := func(id, provider, model, startTime string, multiplier float64) {
		t.Helper()
		if _, err := db.DB().Exec(`
			INSERT INTO requests (id, model, provider, start_time, peak_multiplier) VALUES (?, ?, ?, ?, ?)`,
			id, model, provider, startTime, multiplier); err != nil {
			t.Fatal(err)
		}
	}
	multiplierOf := func(id string) float64 {
		t.Helper()
		var got float64
		if err := db.DB().QueryRow(`SELECT peak_multiplier FROM requests WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	insert("peak-morning", "opencode-go", "deepseek-v4-flash", "2026-09-07T01:30:00Z", 1)
	insert("peak-late", "opencode-go", "deepseek-v4-flash", "2026-09-07T08:00:00Z", 1)
	insert("offpeak-noon", "opencode-go", "deepseek-v4-flash", "2026-09-07T12:00:00Z", 1)
	insert("weekend", "opencode-go", "deepseek-v4-flash", "2026-09-06T08:00:00Z", 1)
	insert("other-model", "opencode-go", "kimi-k2.6", "2026-09-07T08:00:00Z", 1)
	// CommandCode bills the same window, so its DeepSeek rows need the same
	// restoration - this is the case the column was wrong for in production.
	insert("commandcode-peak", "commandcode", "deepseek/deepseek-v4-flash", "2026-09-07T08:00:00Z", 1)
	// A model CommandCode publishes without the peak sub-line.
	insert("commandcode-fast", "commandcode", "deepseek/deepseek-v4-flash-fast", "2026-09-07T08:00:00Z", 1)
	// Another platform has no peak schedule at all.
	insert("other-provider", "openrouter", "deepseek/deepseek-v4-flash", "2026-09-07T08:00:00Z", 1)
	// Already marked: the platform billed it peak from a clock a second or two
	// away from our start_time, and its figure must survive.
	insert("already-peak", "opencode-go", "deepseek-v4-flash", "2026-09-07T10:00:30Z", 2)

	updated, err := db.BackfillPeakMultipliers(context.Background())
	if err != nil || updated != 3 {
		t.Fatalf("BackfillPeakMultipliers = %d, %v; want 3, nil", updated, err)
	}
	for id, want := range map[string]float64{
		"peak-morning": 2, "peak-late": 2, "commandcode-peak": 2,
		"offpeak-noon": 1, "weekend": 1, "other-model": 1,
		"commandcode-fast": 1, "other-provider": 1, "already-peak": 2,
	} {
		if got := multiplierOf(id); got != want {
			t.Errorf("%s peak_multiplier = %v, want %v", id, got, want)
		}
	}

	// Idempotent: a second pass finds nothing left to restore.
	if updated, err := db.BackfillPeakMultipliers(context.Background()); err != nil || updated != 0 {
		t.Fatalf("second run = %d, %v; want 0, nil", updated, err)
	}
}
