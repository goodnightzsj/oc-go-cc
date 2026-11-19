package storage

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		days int
		want int
	}{
		{name: "disabled", days: -1, want: 3},
		{name: "default", days: 0, want: 2},
		{name: "configured", days: 1, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig.WithOverlay(Overlay{
				DatabasePath:  filepath.Join(t.TempDir(), "retention.db"),
				RetentionDays: tc.days,
			})
			db, err := Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			for _, age := range []int{0, 2, 10} {
				// The one spelling the table carries: InsertRequest leaves
				// created_at to the column DEFAULT, and Open normalizes any
				// legacy RFC3339 row once. Retention is then a plain indexed
				// comparison.
				stamp := time.Now().AddDate(0, 0, -age).UTC().Format("2006-01-02 15:04:05")
				if _, err := db.DB().Exec(`INSERT INTO requests (id, model, start_time, created_at) VALUES (?, ?, ?, ?)`, fmt.Sprint(age), "test-model", stamp, stamp); err != nil {
					t.Fatal(err)
				}
			}
			retention := NewRetention(db, cfg.RetentionDays)
			retention.Start()
			defer retention.Stop()
			if tc.days < 0 {
				select {
				case <-retention.doneCh:
				case <-time.After(time.Second):
					t.Fatal("disabled retention must exit without scheduling cleanup")
				}
			}
			deadline := time.Now().Add(time.Second)
			for {
				var count int
				if err := db.DB().QueryRow(`SELECT COUNT(*) FROM requests`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count == tc.want {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("retained rows = %d, want %d", count, tc.want)
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

// Older versions let the provider usage sync write RFC3339Nano into
// created_at. Open normalizes those rows once, so the retention comparison is
// correct and can use the index again.
func TestOpenNormalizesLegacyRequestTimestamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	cfg := DefaultConfig.WithOverlay(Overlay{DatabasePath: path})

	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO requests (id, model, start_time, created_at) VALUES ('legacy','m','2026-01-02T03:04:05.123456789Z','2026-01-02T03:04:05.123456789Z')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	// Compare in SQL: scanning a TIMESTAMP column into a Go string goes
	// through the driver's time formatting and would not show the stored text.
	var normalized bool
	if err := reopened.DB().QueryRow(`SELECT created_at = ? FROM requests WHERE id = 'legacy'`, "2026-01-02 03:04:05").Scan(&normalized); err != nil {
		t.Fatal(err)
	}
	if !normalized {
		t.Fatal("legacy RFC3339 timestamp was not normalized to the column's UTC spelling")
	}
}
