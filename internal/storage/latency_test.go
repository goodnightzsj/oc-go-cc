package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLatencyScopesProviderAndKnownDetails(t *testing.T) {
	db := newCostTestDB(t)
	for _, row := range []struct {
		id, start string
		provider  any
		duration  int
		known     int
		success   any
	}{
		{"commandcode-utc", "2026-09-10T03:00:00Z", "commandcode", 100, 1, 1},
		{"commandcode-offset", "2026-09-10T11:00:00+08:00", "commandcode", 300, 1, 1},
		{"commandcode-zero", "2026-09-10T03:00:00Z", "commandcode", 0, 1, 0},
		{"commandcode-old", "2026-09-10T10:00:00+08:00", "commandcode", 2000, 1, 0},
		{"openrouter", "2026-09-10T03:00:00Z", "openrouter", 900, 1, 1},
		{"legacy-null", "2026-09-10T03:00:00Z", nil, 500, 1, 1},
		{"legacy-empty", "2026-09-10T03:00:00Z", "", 700, 1, 0},
		{"imported-null", "2026-09-10T03:00:00Z", "commandcode", 0, 0, nil},
		{"imported-success", "2026-09-10T03:00:00Z", "commandcode", 1500, 0, 1},
	} {
		if _, err := db.DB().Exec(`INSERT INTO requests
			(id, provider, model, start_time, duration_ms, details_known, success)
			VALUES (?, ?, 'org/shared-model', ?, ?, ?, ?)`,
			row.id, row.provider, row.start, row.duration, row.known, row.success); err != nil {
			t.Fatal(err)
		}
	}

	want := map[string]struct{ count, avgMS, success, failure int64 }{
		"commandcode": {2, 200, 2, 1},
		"openrouter":  {1, 900, 1, 0},
		"":            {2, 600, 1, 1},
	}
	latency := NewLatency(db)
	since := time.Date(2026, 9, 10, 2, 30, 0, 0, time.UTC)
	for _, zone := range []*time.Location{time.UTC, time.FixedZone("UTC+8", 8*60*60)} {
		t.Run(zone.String(), func(t *testing.T) {
			stats, err := latency.GetStats(since.In(zone))
			if err != nil {
				t.Fatal(err)
			}
			if len(stats) != len(want) {
				t.Fatalf("provider groups = %+v, want %d", stats, len(want))
			}
			success, failure, err := latency.GetSuccessCounts(since.In(zone))
			if err != nil {
				t.Fatal(err)
			}
			for _, stat := range stats {
				w, ok := want[stat.Provider]
				if !ok || stat.Model != "org/shared-model" || stat.Count != w.count || stat.Avg.Milliseconds() != w.avgMS {
					t.Errorf("unexpected provider stats: %+v", stat)
				}
				key := stat.Provider + "/" + stat.Model
				if success[key] != w.success || failure[key] != w.failure {
					t.Errorf("%s success/failure = %d/%d, want %d/%d", key, success[key], failure[key], w.success, w.failure)
				}
				encoded, err := json.Marshal(stat)
				if err != nil {
					t.Fatal(err)
				}
				var wire map[string]any
				if err := json.Unmarshal(encoded, &wire); err != nil {
					t.Fatal(err)
				}
				if wire["provider"] != stat.Provider || wire["model"] != stat.Model {
					t.Errorf("wire identity = %s, want provider %q/model %q", encoded, stat.Provider, stat.Model)
				}
			}
		})
	}
}

func TestLatencySampleLimitIsPerProviderNotSuccessCount(t *testing.T) {
	db := newCostTestDB(t)
	if _, err := db.DB().Exec(`WITH RECURSIVE samples(n) AS (
		SELECT 1 UNION ALL SELECT n + 1 FROM samples WHERE n < ?
	)
	INSERT INTO requests (id, provider, model, start_time, duration_ms, details_known, success)
	SELECT 'sample-' || n, 'openrouter', 'shared-model', '2026-09-10T12:00:00Z', 100, 1, 1
	FROM samples`, maxSamplesPerModel+1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO requests
		(id, provider, model, start_time, duration_ms, details_known, success)
		VALUES ('other-platform', 'commandcode', 'shared-model', '2026-09-10T11:00:00Z', 900, 1, 1)`); err != nil {
		t.Fatal(err)
	}
	latency := NewLatency(db)
	stats, err := latency.GetStats(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 {
		t.Fatalf("provider stats = %+v, want two independent distributions", stats)
	}
	for _, stat := range stats {
		wantCount, wantAvg := int64(maxSamplesPerModel), 100*time.Millisecond
		if stat.Provider == "commandcode" {
			wantCount, wantAvg = 1, 900*time.Millisecond
		}
		if stat.Count != wantCount || stat.Avg != wantAvg {
			t.Errorf("stats = %+v, want count %d/average %v", stat, wantCount, wantAvg)
		}
	}
	success, failure, err := latency.GetSuccessCounts(time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if success["openrouter/shared-model"] != maxSamplesPerModel+1 || success["commandcode/shared-model"] != 1 || len(failure) != 0 {
		t.Fatalf("outcome counts must not be limited by latency sampling: success=%v failure=%v", success, failure)
	}
}

func TestCalculateStatsOddMedian(t *testing.T) {
	stat := calculateStats("model", []int64{30, 10, 20})
	if stat.P50 != 20*time.Millisecond {
		t.Fatalf("P50 = %v, want the middle sample 20ms", stat.P50)
	}
}

func TestParseTimeRangeNinetyDays(t *testing.T) {
	before := time.Now().Add(-90 * 24 * time.Hour)
	got := ParseTimeRange("90d")
	after := time.Now().Add(-90 * 24 * time.Hour)
	if got.Before(before) || got.After(after) {
		t.Fatalf("90d range = %v, want a 90-day boundary", got)
	}
}
