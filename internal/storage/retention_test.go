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
			defer db.Close()
			for _, age := range []int{0, 2, 10} {
				when := time.Now().AddDate(0, 0, -age).UTC().Format(time.RFC3339Nano)
				if _, err := db.DB().Exec(`INSERT INTO requests (id, model, start_time, created_at) VALUES (?, ?, ?, ?)`, fmt.Sprint(age), "test-model", when, when); err != nil {
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
