package gui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

// The dashboard shows a Peak badge from its own copy of the peak schedule,
// because it must render a row the backfill has not reached yet. Go owns the
// rule and the frontend mirrors it, exactly as the platform list does - and
// this is the assertion that keeps the mirror honest.
//
// The frontend copy is the dangerous one to drift: a model added to the backend
// but not here renders as off-peak, and the row looks perfectly well-formed, so
// the difference is invisible until someone compares a bill.
var (
	peakFamiliesRe = regexp.MustCompile(`(?s)const PEAK_FAMILIES = \[(.*?)\];`)
	peakScheduleRe = regexp.MustCompile(`'?([a-z0-9-]+)'?:\s*\{models:\s*PEAK_FAMILIES,\s*windows:\s*(\[\[.*?\]\])[^}]*multiplier:\s*([0-9.]+)\}`)
	peakFamilyItem = regexp.MustCompile(`'([^']+)'`)
)

func jsPeakSchedule(t *testing.T) (families []string, schedules map[string]struct {
	windows    [][2]int
	multiplier float64
}) {
	t.Helper()
	app := readEmbeddedAsset(t, "assets/app.js")

	fm := peakFamiliesRe.FindStringSubmatch(app)
	if fm == nil {
		t.Fatal("app.js has no PEAK_FAMILIES list")
	}
	for _, m := range peakFamilyItem.FindAllStringSubmatch(fm[1], -1) {
		families = append(families, m[1])
	}

	schedules = map[string]struct {
		windows    [][2]int
		multiplier float64
	}{}
	pairRe := regexp.MustCompile(`\[(\d+),\s*(\d+)\]`)
	for _, m := range peakScheduleRe.FindAllStringSubmatch(app, -1) {
		var h struct {
			windows    [][2]int
			multiplier float64
		}
		for _, p := range pairRe.FindAllStringSubmatch(m[2], -1) {
			h.windows = append(h.windows, [2]int{atoi(p[1]), atoi(p[2])})
		}
		h.multiplier = atof(m[3])
		schedules[m[1]] = h
	}
	return families, schedules
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func atof(s string) float64 {
	whole, frac, hasFrac := 0.0, 0.0, false
	div := 1.0
	for _, c := range s {
		if c == '.' {
			hasFrac = true
			continue
		}
		if hasFrac {
			div *= 10
			frac += float64(c-'0') / div
			continue
		}
		whole = whole*10 + float64(c-'0')
	}
	return whole + frac
}

// The two schedules must cover the same platforms and the same window hours:
// they answer the same question about the same money.
func TestPeakSchedulesMatchTheBackend(t *testing.T) {
	families, schedules := jsPeakSchedule(t)

	// Every family the frontend lists must be peak-priced by the backend, and
	// every family the backend covers must be listed here.
	inside := time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC) // Monday 02:00 UTC
	for _, family := range families {
		if got := history.ProviderPeakMultiplier("opencode-go", family, inside); got <= 1 {
			t.Errorf("app.js lists %q but the backend does not peak-price it", family)
		}
	}
	for family := range history.PeakModelFamilies() {
		if !contains(families, family) {
			t.Errorf("backend peak-prices %q but app.js does not list it, so its rows would render off-peak", family)
		}
	}

	// Only the platforms that actually publish peak pricing may appear, and
	// each must use the backend's window and multiplier.
	for provider, s := range schedules {
		wantWindow, wantMul := history.PeakSchedule(provider)
		if wantMul <= 1 {
			t.Errorf("app.js peak-prices %q but the backend has no schedule for it", provider)
			continue
		}
		if s.multiplier != wantMul {
			t.Errorf("%s multiplier: app.js %v, backend %v", provider, s.multiplier, wantMul)
		}
		if len(s.windows) != len(wantWindow) {
			t.Errorf("%s windows: app.js %v, backend %v", provider, s.windows, wantWindow)
			continue
		}
		for i, w := range wantWindow {
			if s.windows[i] != [2]int{w.Start, w.End} {
				t.Errorf("%s window %d: app.js %v, backend [%d,%d)", provider, i, s.windows[i], w.Start, w.End)
			}
		}
	}
	for provider := range history.PeakScheduledProviders() {
		if _, ok := schedules[provider]; !ok {
			t.Errorf("backend has a schedule for %q that app.js does not, so its rows would never show a badge", provider)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}
