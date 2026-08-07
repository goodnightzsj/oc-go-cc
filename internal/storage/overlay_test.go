package storage

import "testing"

// TestWithOverlay_PartialConfigKeepsDefaults is the regression guard for a bug
// that silently disabled the entire persistence layer. Callers used to build a
// Config wholesale from the user's config file, so a file that set only
// analytics_baseline produced an empty DatabasePath — and an empty path makes
// the caller skip Open entirely, taking SQLite history and every
// /api/analytics/* endpoint down with it. The endpoints then fell through to
// the static-asset catch-all and answered with index.html instead of JSON.
func TestWithOverlay_PartialConfigKeepsDefaults(t *testing.T) {
	got := DefaultConfig.WithOverlay(Overlay{AnalyticsBaseline: "2026-08-06T12:07:00Z"})

	if got.DatabasePath != DefaultConfig.DatabasePath {
		t.Errorf("DatabasePath = %q, want the default %q (an empty path disables persistence)",
			got.DatabasePath, DefaultConfig.DatabasePath)
	}
	if got.RetentionDays != DefaultConfig.RetentionDays {
		t.Errorf("RetentionDays = %d, want the default %d", got.RetentionDays, DefaultConfig.RetentionDays)
	}
	if !got.WALEnabled {
		t.Error("WALEnabled = false, want the default true")
	}
	if got.AnalyticsBaseline != "2026-08-06T12:07:00Z" {
		t.Errorf("AnalyticsBaseline = %q, want it applied", got.AnalyticsBaseline)
	}
}

// TestWithOverlay_EmptyOverlayChangesNothing pins the zero value's meaning:
// a config file with no storage section must leave the defaults alone.
func TestWithOverlay_EmptyOverlayChangesNothing(t *testing.T) {
	if got := DefaultConfig.WithOverlay(Overlay{}); got != DefaultConfig {
		t.Errorf("empty overlay changed the config:\n got %+v\nwant %+v", got, DefaultConfig)
	}
}

// TestWithOverlay_SetFieldsWin covers the normal path, including the two fields
// whose zero value is meaningful. WALEnabled defaults to true, so a plain bool
// could never express "off"; it takes a pointer, and an explicit false must
// survive. VacuumOnStartup defaults to false, so only true is distinguishable.
func TestWithOverlay_SetFieldsWin(t *testing.T) {
	off := false
	got := DefaultConfig.WithOverlay(Overlay{
		DatabasePath:    "/tmp/custom.db",
		RetentionDays:   30,
		VacuumOnStartup: true,
		WALEnabled:      &off,
	})

	if got.DatabasePath != "/tmp/custom.db" {
		t.Errorf("DatabasePath = %q, want /tmp/custom.db", got.DatabasePath)
	}
	if got.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30", got.RetentionDays)
	}
	if !got.VacuumOnStartup {
		t.Error("VacuumOnStartup = false, want true")
	}
	if got.WALEnabled {
		t.Error("WALEnabled = true, want an explicit false to override the default")
	}
}
