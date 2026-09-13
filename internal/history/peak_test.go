package history

import (
	"testing"
	"time"
)

// inside is a Monday 02:00 UTC instant: weekday, inside the 01-04 window.
var inside = time.Date(2026, 9, 7, 2, 0, 0, 0, time.UTC)

// Both platforms publish the same four peak-priced models and name them row by
// row, so a family-wide substring match is wrong in both directions: it would
// peak-price the older DeepSeek families and the "fast" variant, none of which
// carry a second rate. Re-verified against the live pages 2026-09-14.
func TestPeakCoversOnlyTheDocumentedFamilies(t *testing.T) {
	peakPriced := []string{
		"deepseek-v4.1-flash",
		"deepseek-v4-flash",
		"deepseek-v4-flash-vision-exp",
		"deepseek-v4-pro",
		// The same models as CommandCode namespaces them.
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4.1-flash",
	}
	// Single-rate models: the older families, the "fast" variant the pricing
	// page lists without a peak sub-line, and dated snapshots.
	singleRate := []string{
		"deepseek-v3", "deepseek-v3.2", "deepseek-r1", "deepseek-chat",
		"deepseek-reasoner",
		"deepseek/deepseek-v4-flash-fast",
		"deepseek-v4-flash-0423", "deepseek-v4-pro-0813",
		"kimi-k2.6", "glm-5.2", "moonshotai/Kimi-K2.6",
	}
	for _, provider := range []string{"opencode-go", "commandcode"} {
		for _, model := range peakPriced {
			if got := ProviderPeakMultiplier(provider, model, inside); got != 2 {
				t.Errorf("%s/%s = %v, want 2", provider, model, got)
			}
		}
		for _, model := range singleRate {
			if got := ProviderPeakMultiplier(provider, model, inside); got != 1 {
				t.Errorf("%s/%s = %v, want 1 (not peak-priced)", provider, model, got)
			}
		}
	}
}

// The window is weekday-only and half-open at both ends, so 04:00 and 06:00 are
// off-peak and 10:00 ends it. Read in UTC - the instant, not the wall clock of
// whoever is looking at the dashboard.
func TestPeakWindowBoundariesAreHalfOpenAndUTC(t *testing.T) {
	mk := func(day time.Weekday, hour int) time.Time {
		// 2026-09-07 is a Monday.
		base := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
		return base.AddDate(0, 0, int(day)-int(time.Monday)).Add(time.Duration(hour) * time.Hour)
	}
	west := time.FixedZone("UTC-05", -5*60*60)
	for _, c := range []struct {
		name string
		t    time.Time
		want float64
	}{
		{"mon 01:00 opens", mk(time.Monday, 1), 2},
		{"mon 03:59 inside", mk(time.Monday, 3), 2},
		{"mon 04:00 closes", mk(time.Monday, 4), 1},
		{"mon 06:00 opens", mk(time.Monday, 6), 2},
		{"mon 09:59 inside", mk(time.Monday, 9), 2},
		{"mon 10:00 closes", mk(time.Monday, 10), 1},
		{"mon 00:59 before", mk(time.Monday, 0), 1},
		{"fri 02:00 weekday", mk(time.Friday, 2), 2},
		{"sat 02:00 weekend", mk(time.Saturday, 2), 1},
		{"sun 08:00 weekend", mk(time.Sunday, 8), 1},
	} {
		for _, stamp := range []time.Time{c.t, c.t.In(west)} {
			if got := ProviderPeakMultiplier("opencode-go", "deepseek-v4-flash", stamp); got != c.want {
				t.Errorf("%s at %v = %v, want %v", c.name, stamp, got, c.want)
			}
		}
	}
	if got := ProviderPeakMultiplier("opencode-go", "deepseek-v4-flash", time.Time{}); got != 1 {
		t.Errorf("zero time = %v, want 1", got)
	}
}

// A platform with no published peak pricing is never peak-priced, and an empty
// provider keeps the pre-provider-column reading (the default platform).
func TestPeakIsScopedToPlatformsThatPublishIt(t *testing.T) {
	for _, provider := range []string{"opencode-zen", "aws-bedrock", "openrouter"} {
		if got := ProviderPeakMultiplier(provider, "deepseek-v4-flash", inside); got != 1 {
			t.Errorf("%s = %v, want 1 (platform publishes no peak pricing)", provider, got)
		}
	}
	if got := ProviderPeakMultiplier("", "deepseek-v4-flash", inside); got != 2 {
		t.Errorf("empty provider = %v, want 2 (legacy records mean the default platform)", got)
	}
	// Legacy underscore spelling normalises to the hyphenated id.
	if got := ProviderPeakMultiplier("opencode_go", "deepseek-v4-flash", inside); got != 2 {
		t.Errorf("opencode_go = %v, want 2", got)
	}
}

// Every schedule must be internally consistent: windows inside a day, and a
// multiplier that actually means "peak". A malformed entry would otherwise
// price whole windows at 1x while looking configured.
func TestEveryPeakScheduleIsWellFormed(t *testing.T) {
	for provider, s := range peakSchedules {
		if len(s.models) == 0 {
			t.Errorf("%s: no models listed", provider)
		}
		if s.multiplier <= 1 {
			t.Errorf("%s: multiplier %v does not mark peak", provider, s.multiplier)
		}
		if len(s.windows) == 0 {
			t.Errorf("%s: no windows", provider)
		}
		for _, w := range s.windows {
			if w.Start < 0 || w.End > 24 || w.Start >= w.End {
				t.Errorf("%s: window [%d,%d) is not a valid hour range", provider, w.Start, w.End)
			}
		}
	}
}
