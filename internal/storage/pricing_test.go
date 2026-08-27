package storage

import (
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
	// costForTokensAt in peak window doubles base cost
	peakT := time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC) // Tuesday 07:00Z
	base := costForTokens("deepseek-v4-flash", 1000, 500, 200000, 0, 0.22, 0.66)
	peak := costForTokensAt("deepseek-v4-flash", 1000, 500, 200000, 0, 0.22, 0.66, peakT)
	if peak != 2*base {
		t.Fatalf("peak cost %v, want 2×base %v", peak, 2*base)
	}
	// input served from cache bills at the cache rate: platform billed a
	// 516638-in/516608-cr/399-out request as (516638-516608)*0.22 + 516608*0.007
	// + 399*0.66, doubled in the 07:00Z peak window -> 777239 units (1e-8 USD).
	cacheOverlap := costForTokensAt("deepseek-v4-flash", 516638, 399, 516608, 0, 0.22, 0.66, peakT)
	if got := int64(math.Round(cacheOverlap * 1e8)); got != 777239 {
		t.Fatalf("cache-overlap cost %v units, want platform 777239", got)
	}
}