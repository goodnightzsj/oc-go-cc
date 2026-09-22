package storage

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/history"
)

func TestPeakMultiplier(t *testing.T) {
	mk := func(day time.Weekday, hour int) time.Time {
		// anchor on a fixed UTC instant of the given weekday/hour
		for base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC); ; base = base.Add(24 * time.Hour) {
			if base.Weekday() == day {
				return time.Date(base.Year(), base.Month(), base.Day(), hour, 30, 0, 0, time.UTC)
			}
		}
	}
	cases := []struct {
		name string
		day  time.Weekday
		hour int
		want float64
	}{
		{"tue peak1", time.Tuesday, 1, 2},
		{"fri peak1-edge-03", time.Friday, 3, 2},
		{"mon peak2", time.Monday, 6, 2},
		{"wed peak2-09", time.Wednesday, 9, 2},
		{"thu off-00", time.Thursday, 0, 1},
		{"thu off-04", time.Thursday, 4, 1},
		{"thu off-05", time.Thursday, 5, 1},
		{"thu off-10", time.Thursday, 10, 1},
		{"sat off-even-peak1", time.Saturday, 2, 1},
		{"sun off-07", time.Sunday, 7, 1},
	}
	for _, c := range cases {
		got := history.PeakMultiplier("deepseek-v4-flash", mk(c.day, c.hour))
		if got != c.want {
			t.Errorf("%s: peakMultiplier = %v, want %v", c.name, got, c.want)
		}
	}
	// non-deepseek model is never peaked
	if got := history.PeakMultiplier("kimi-k2.6", mk(time.Tuesday, 7)); got != 1 {
		t.Errorf("kimi peak multiplier = %v, want 1", got)
	}
	// zero time degrades to off-peak
	if got := history.PeakMultiplier("deepseek-v4-flash", time.Time{}); got != 1 {
		t.Errorf("zero-time multiplier = %v, want 1", got)
	}
}

func TestParseRequestTime(t *testing.T) {
	// +08:00 stored format
	tz := parseRequestTime("2026-08-26T17:59:21.93350689+08:00")
	if tz.IsZero() || tz.Hour() != 17 {
		t.Fatalf("parse +08:00 failed: %v", tz)
	}
	// Z suffix (platform raw)
	z := parseRequestTime("2026-08-26T09:59:21.93350689Z")
	if z.IsZero() || z.Hour() != 9 {
		t.Fatalf("parse Z failed: %v", z)
	}
	// garbage -> zero
	if !parseRequestTime("garbage").IsZero() {
		t.Fatal("garbage should parse to zero time")
	}
	if !parseRequestTime("").IsZero() {
		t.Fatal("empty should parse to zero time")
	}
	// costForProviderTokensAt in the peak window doubles the off-peak cost
	peakT := time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC)     // Tuesday 07:00Z, deepseek ×2
	offPeakT := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) // Tuesday 12:00Z, deepseek ×1
	base, ok := costForProviderTokensAt("opencode-go", "deepseek-v4-flash", 1000, 500, 200000, 0,
		sql.NullFloat64{}, sql.NullFloat64{}, offPeakT)
	if !ok {
		t.Fatal("deepseek-v4-flash on opencode-go must be priced")
	}
	peak, ok := costForProviderTokensAt("opencode-go", "deepseek-v4-flash", 1000, 500, 200000, 0,
		sql.NullFloat64{}, sql.NullFloat64{}, peakT)
	if !ok {
		t.Fatal("deepseek-v4-flash on opencode-go must be priced in the peak window too")
	}
	if peak != 2*base {
		t.Fatalf("peak cost %v, want 2×base %v", peak, 2*base)
	}
	// Platform never deducts cache from input: `in` is the cache-miss part
	// already, billed at the full input rate; the hit part bills at the cache
	// rate on top. This is the structural claim, and it holds at whatever rates
	// are current, so it is asserted against the table rather than a literal.
	//
	// The rates themselves changed: an invoice on 2026-08-28 priced
	// 18355-in/18176-cr/231-out at 0.00863558, which the old 0.22/0.007/0.66
	// reproduces exactly and today's 0.15/0.003/0.60 does not. Recompute that
	// invoice from the current table and it disagrees - which is why historical
	// rows keep their stored cost and are never re-priced.
	const in, out, cacheRead = 516638, 399, 516608
	ipm, opm, crpm, _, _ := PriceForProviderModel("opencode-go", "deepseek-v4-flash", in+cacheRead)
	want := (float64(in)*ipm + float64(cacheRead)*crpm + float64(out)*opm) / 1e6 * 2
	cacheOverlap, ok := costForProviderTokensAt("opencode-go", "deepseek-v4-flash", in, out, cacheRead, 0,
		sql.NullFloat64{}, sql.NullFloat64{}, peakT)
	if !ok {
		t.Fatal("deepseek-v4-flash on opencode-go must be priced")
	}
	if math.Abs(cacheOverlap-want) > 1e-12 {
		t.Fatalf("cache-overlap cost %v, want %v (miss at input + hit at cache_read, doubled)", cacheOverlap, want)
	}
	// A deduction of the cache prefix would price this lower; that was the bug.
	deducted := (float64(in-cacheRead)*ipm + float64(cacheRead)*crpm + float64(out)*opm) / 1e6 * 2
	if math.Abs(cacheOverlap-deducted) < 1e-12 {
		t.Fatal("cost equals the cache-deducted formula, so the miss part is being under-billed again")
	}
}

// OpenRouter's hy3 is the first peak rule in this codebase whose multiplier is
// not a whole number, and a fractional multiplier is the one value that could
// survive rule evaluation and still be lost on the way to a stored cost - a
// table typed as "peak means double", a rounding step, an int cast. The rule
// itself is covered by history's TestOpenRouterPeakMatchesThePublishedOverrides;
// this drives the same hour through the pricing path that actually writes
// cost_usd, so 1.6 has to be carried by the money, not just by the rule.
//
// OpenRouter is priced from the catalog, so rates arrive as arguments here
// rather than from a platform table; any pair works, and the assertion is
// against the off-peak cost of the same tokens.
func TestOpenRouterFractionalPeakSurvivesTheCostPath(t *testing.T) {
	in := sql.NullFloat64{Float64: 0.0825, Valid: true} // hy3's catalog input rate
	out := sql.NullFloat64{Float64: 0.33, Valid: true}  // and its output rate
	const tokens = 1_000_000

	// Monday 03:00Z and Sunday 09:00Z are both inside 0000-1600, which hy3 bills
	// every day of the week; Monday 20:00Z is outside it.
	cost := func(at time.Time) float64 {
		got, ok := costForProviderTokensAt("openrouter", "tencent/hy3", tokens, tokens, 0, 0, in, out, at)
		if !ok {
			t.Fatalf("openrouter/tencent-hy3 must be priced at %v", at)
		}
		return got
	}
	offPeak := cost(time.Date(2026, 9, 7, 20, 0, 0, 0, time.UTC))
	if math.Abs(offPeak-(1_000_000*0.0825+1_000_000*0.33)/1e6) > 1e-12 {
		t.Fatalf("off-peak cost %v is not 1M in plus 1M out at the given rates", offPeak)
	}
	for _, stamp := range []time.Time{
		time.Date(2026, 9, 7, 3, 0, 0, 0, time.UTC), // Monday, inside
		time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC), // Sunday, inside: no day condition
	} {
		if got := cost(stamp); math.Abs(got-1.6*offPeak) > 1e-12 {
			t.Errorf("cost at %v = %v, want 1.6×off-peak %v", stamp, got, 1.6*offPeak)
		}
	}
}

// CommandCode prints the peak sub-line on individual model rows rather than
// covering a whole family, so the covered set has to be exactly what it
// publishes - a substring match would wrongly peak-price the variants that are
// listed without it. Source: commandcode.ai/docs/plans/goat (peak 01-04 &
// 06-10 UTC, Mon-Fri).
func TestCommandCodePeakCoversOnlyPublishedModels(t *testing.T) {
	peak := time.Date(2026, 9, 7, 1, 30, 0, 0, time.UTC) // Monday, inside the window
	for model, want := range map[string]float64{
		"deepseek/deepseek-v4.1-flash":          2,
		"deepseek/deepseek-v4-flash":            2,
		"deepseek/deepseek-v4-flash-vision-exp": 2,
		"deepseek/deepseek-v4-pro":              2,
		// Served by the API but published without the peak sub-line.
		"deepseek/deepseek-v4-flash-fast": 1,
		"moonshotai/Kimi-K2.6":            1,
	} {
		if got := history.ProviderPeakMultiplier("commandcode", model, peak); got != want {
			t.Errorf("commandcode %s = %v, want %v", model, got, want)
		}
	}
	for _, stamp := range []time.Time{
		time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC), // weekday, outside the window
		time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC),  // Sunday, inside the hours
	} {
		if got := history.ProviderPeakMultiplier("commandcode", "deepseek/deepseek-v4-flash", stamp); got != 1 {
			t.Errorf("commandcode at %v = %v, want 1", stamp, got)
		}
	}
}
