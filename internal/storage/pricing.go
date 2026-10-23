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

// costForTokens prices a token mix in USD using the platform's own published
// rates. Callers must already have established that the platform publishes a
// rate for this model; complete request estimates go through
// costForProviderTokensAt, which retains unknown-price semantics.
func costForTokens(provider, model string, in, out, cacheRead, cacheCreate int64) float64 {
	// The published context bands are measured against the whole prompt, not
	// the cache-miss part alone: a request that reads 500K cached tokens and
	// misses on 1K still bills in the > 256K band.
	ipm, opm, crpm, cwpm, _ := PriceForProviderModel(provider, model, in+cacheRead+cacheCreate)
	// Cache semantics: cache CREATION (first-time prompt write) bills at the
	// cache_write price when the model publishes one, otherwise at the input
	// price; cache READ bills at the cheap cache_read price.
	// `in` is the cache-miss part already (splitPromptTokens strips the cached
	// prefix before storage), so no deduction applies. Verified against the
	// platform invoice: bill = miss*input + hit*cache_read + out*output,
	// never miss-cacheRead (that under-prices rows where miss > hit).
	cacheWriteRate := ipm
	if cwpm > 0 {
		cacheWriteRate = cwpm
	}
	return (float64(in)*ipm +
		float64(cacheCreate)*cacheWriteRate +
		float64(cacheRead)*crpm +
		float64(out)*opm) / 1_000_000
}

// costForProviderTokensAt returns a complete estimate only when every consumed
// token category has a known rate. Catalog prices are provider-specific and do
// not include cache rates; a missing rate must not be interpreted as free usage.
func costForProviderTokensAt(provider, model string, in, out, cacheRead, cacheCreate int64, inputRate, outputRate sql.NullFloat64, t time.Time, tiers []PromptTier) (float64, bool) {
	// Which rate source and cache-creation rule apply is platform metadata, not
	// something to infer from the provider's name here. An unknown provider
	// resolves to no descriptor, which leaves the table empty - the same
	// conservative answer the name comparison gave.
	descriptor, _ := site.Lookup(provider)
	if descriptor.RateTable != "" {
		if _, _, _, _, ok := PriceForProviderModel(provider, model, in+cacheRead+cacheCreate); ok {
			return costForTokens(provider, model, in, out, cacheRead, cacheCreate) *
				history.ProviderPeakMultiplier(provider, model, t), true
		}
	}
	// Insert and historical backfill select catalog bands from the same whole
	// prompt. Published platform tables above retain their own rate precedence.
	if band := tierRatesFor(tiers, in+cacheRead+cacheCreate); band != nil {
		inputRate = sql.NullFloat64{Float64: band.Input, Valid: true}
		outputRate = sql.NullFloat64{Float64: band.Output, Valid: true}
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
