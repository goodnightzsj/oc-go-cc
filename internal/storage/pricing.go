package storage

import (
	"database/sql"
	"time"

	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/site"
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

// costForProviderTokensAt returns a complete estimate only when every consumed
// token category has a known rate. Catalog prices are provider-specific and do
// not include cache rates; a missing rate must not be interpreted as free usage.
func costForProviderTokensAt(provider, model string, in, out, cacheRead, cacheCreate int64, inputRate, outputRate sql.NullFloat64, t time.Time) (float64, bool) {
	// Which rate source and cache-creation rule apply is platform metadata, not
	// something to infer from the provider's name here. An unknown provider
	// resolves to no descriptor, which leaves the table empty - the same
	// conservative answer the name comparison gave.
	descriptor, _ := site.Lookup(provider)
	if descriptor.RateTable != "" {
		if _, _, _, _, ok := PriceForProviderModel(provider, model); ok {
			return costForTokens(provider, model, in, out, cacheRead, cacheCreate) *
				history.ProviderPeakMultiplier(provider, model, t), true
		}
	}
	if !inputRate.Valid || !outputRate.Valid || cacheRead != 0 || (!descriptor.CacheCreationBilledAsInput && cacheCreate != 0) {
		return 0, false
	}
	input := in
	if descriptor.CacheCreationBilledAsInput {
		input += cacheCreate
	}
	cost := (float64(input)*inputRate.Float64 + float64(out)*outputRate.Float64) / 1_000_000
	return cost * history.ProviderPeakMultiplier(provider, model, t), true
}
