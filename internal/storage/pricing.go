package storage

import (
	"time"

	"github.com/routatic/proxy/internal/history"
)

// parseRequestTime parses the ISO-8601 request timestamp stored in
// requests.start_time (either "+08:00" or "Z" suffix); returns zero time if
// unparsable so peak pricing degrades to off-peak (multiplier 1).
func parseRequestTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2
		}
		return time.Time{}
	}
	return t
}

// costForTokensAt prices a request at time t, applying the provider peak
// multiplier (history.PeakMultiplier) on top of the base seeded rates.
// Aggregations that cannot see individual timestamps keep using costForTokens
// (off-peak base rates).
func costForTokensAt(model string, in, out, cacheRead, cacheCreate int64, modelsInputPerM, modelsOutputPerM float64, t time.Time) float64 {
	return costForTokens(model, in, out, cacheRead, cacheCreate, modelsInputPerM, modelsOutputPerM) * history.PeakMultiplier(model, t)
}