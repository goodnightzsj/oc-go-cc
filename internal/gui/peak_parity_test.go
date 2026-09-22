package gui

import (
	"regexp"
	"strings"
	"testing"
)

// The dashboard used to carry its own copy of the peak schedule, and this file
// kept that copy honest. The copy is gone, and this is the assertion that keeps
// it gone: the browser has no holiday calendar, so a re-derived multiplier would
// badge a Chinese public holiday at peak while the backend billed it off-peak.
//
// The rule itself has one owner - history.peakSchedules - and one test,
// internal/history/holidays_test.go.
func TestDashboardDoesNotReDerivePeakPricing(t *testing.T) {
	app := readEmbeddedAsset(t, "assets/app.js")

	for _, banned := range []struct{ needle, why string }{
		{"PEAK_SCHEDULES", "a second copy of the schedule would disagree with the backend's holiday calendar"},
		{"PEAK_FAMILIES", "a second copy of the covered models would drift from the backend's"},
		{"getUTCDay", "re-deriving the day would badge holidays at peak"},
	} {
		if strings.Contains(app, banned.needle) {
			t.Errorf("app.js still carries %q: %s", banned.needle, banned.why)
		}
	}

	// The read itself must survive: the stored column is the badge's only input.
	if !strings.Contains(app, "peak_multiplier") {
		t.Error("app.js no longer reads peak_multiplier, so no row can show a peak badge")
	}
}

// TestEffectivePeakMultiplierReadsOnlyTheStoredColumn pins the function's
// contract: the backend's figure decides, and nothing about the time of day,
// the provider or the model may enter into it. Any of those would be a
// re-derivation, and a re-derivation is wrong on every holiday.
func TestEffectivePeakMultiplierReadsOnlyTheStoredColumn(t *testing.T) {
	app := readEmbeddedAsset(t, "assets/app.js")
	re := regexp.MustCompile(`(?s)function effectivePeakMultiplier\(h\)\s*\{(.*?)\n\}`)
	m := re.FindStringSubmatch(app)
	if m == nil {
		t.Fatal("effectivePeakMultiplier is not defined in app.js")
	}
	for _, banned := range []string{"start_time", "Date(", "provider", "model"} {
		if strings.Contains(m[1], banned) {
			t.Errorf("effectivePeakMultiplier reads %q; it must use only the stored column", banned)
		}
	}
}
