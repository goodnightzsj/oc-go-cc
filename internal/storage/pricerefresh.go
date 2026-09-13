package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/routatic/proxy/internal/site"
)

// Published prices are refreshed from the platforms' own pages rather than
// only shipped as a build-time snapshot, because the snapshot proved to be
// wrong within weeks: OpenCode Go moved its DeepSeek rows from 0.22/0.66 to
// 0.15/0.60 and CommandCode repriced four models, and nothing reported it -
// the dashboard simply showed well-formed numbers computed from retired rates.
//
// Two sources, chosen because they are the ones that actually publish a full
// per-model table in a parseable form:
//
//   - OpenCode Go: the same models.dev catalog this proxy already syncs. Its
//     providers["opencode-go"].models carries cost and context tiers. Read from
//     the local synced file, so refreshing costs no extra network request and
//     the daily catalog sync stays the only thing that talks to models.dev.
//   - CommandCode: the plan page's embedded payload. Its API publishes model
//     ids but no prices, and /models renders client-side, so the embedded JSON
//     is the only machine-readable table.
//
// A refresh that fails for one platform leaves that platform's previous table
// in place; it never installs a partial or empty table over a good one.

const (
	// CommandCodePricesURL is the plan page carrying the full per-model table.
	// Every plan page that shows the table (goat, pro) carries the same 70 rows
	// at identical rates, so one is enough.
	CommandCodePricesURL = "https://commandcode.ai/docs/plans/pro"

	// DefaultPriceRefreshInterval matches the published cadence of both pages:
	// neither has a documented update schedule, and price moves are rare, so an
	// hour is responsiveness without being abusive.
	DefaultPriceRefreshInterval = time.Hour

	priceFetchTimeout = 30 * time.Second
	maxPricePageBytes = 8 << 20
)

// priceOverrides holds the last successfully refreshed table per platform. A
// nil map means "nothing refreshed yet, use the embedded seed". In memory only:
// a restart re-fetches, and the embedded seed is now accurate enough to serve
// in the meantime if that fetch fails.
var priceOverrides atomic.Pointer[map[string][]priceEntry]

// tableFor returns the entries to price from: the refreshed table when one has
// been installed for this platform, otherwise the embedded seed. ok is false
// only when the platform publishes no table at all, which is a different
// answer from "a table with no matching rule" - the caller distinguishes an
// unpriced platform from an unpriced model.
func tableFor(rateTable string) ([]priceEntry, bool) {
	if p := priceOverrides.Load(); p != nil {
		if entries, ok := (*p)[rateTable]; ok && len(entries) > 0 {
			return entries, true
		}
	}
	tables, err := rateTables()
	if err != nil {
		return nil, false
	}
	entries, ok := tables[rateTable]
	return entries, ok
}

// InstallPrices replaces the live price tables. Entries for a platform absent
// from the map are left as they are, so a refresh that only reached one
// platform cannot blank the other.
func InstallPrices(fetched map[string][]priceEntry) {
	if len(fetched) == 0 {
		return
	}
	current := map[string][]priceEntry{}
	if p := priceOverrides.Load(); p != nil {
		for k, v := range *p {
			current[k] = v
		}
	}
	for k, v := range fetched {
		if len(v) > 0 {
			current[k] = v
		}
	}
	priceOverrides.Store(&current)
}

// CurrentPrices reports which platforms have a refreshed table installed, and
// how many rules each holds. Callers use it to show whether prices are live or
// still the build-time snapshot.
func CurrentPrices() map[string]int {
	out := map[string]int{}
	if p := priceOverrides.Load(); p != nil {
		for k, v := range *p {
			out[k] = len(v)
		}
	}
	return out
}

// modelsDevProvider is the slice of a models.dev catalog entry this proxy
// prices from. Only the fields the price table needs are decoded.
type modelsDevProvider struct {
	Models map[string]struct {
		Cost *struct {
			Input      *float64 `json:"input"`
			Output     *float64 `json:"output"`
			CacheRead  *float64 `json:"cache_read"`
			CacheWrite *float64 `json:"cache_write"`
			Tiers      []struct {
				Input      float64  `json:"input"`
				Output     float64  `json:"output"`
				CacheRead  *float64 `json:"cache_read"`
				CacheWrite *float64 `json:"cache_write"`
				Tier       *struct {
					Size *int64 `json:"size"`
				} `json:"tier"`
			} `json:"tiers"`
		} `json:"cost"`
	} `json:"models"`
}

// FetchOpenCodeGoPrices reads the OpenCode Go model table out of a locally
// synced models.dev catalog. An empty path, a missing file, or a catalog
// without that provider are all reported as errors so the caller keeps the
// table it already has.
func FetchOpenCodeGoPrices(catalogPath string) ([]priceEntry, error) {
	if catalogPath == "" {
		return nil, fmt.Errorf("no catalog path configured")
	}
	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("read catalog: %w", err)
	}
	var envelope struct {
		Providers map[string]modelsDevProvider `json:"providers"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	provider, ok := envelope.Providers[site.OpenCodeGo]
	if !ok {
		return nil, fmt.Errorf("catalog has no %s provider", site.OpenCodeGo)
	}
	entries := make([]priceEntry, 0, len(provider.Models))
	for id, m := range provider.Models {
		if m.Cost == nil || m.Cost.Input == nil || m.Cost.Output == nil {
			continue // a model with no published price is not a zero-price model
		}
		e := priceEntry{Match: id, Input: *m.Cost.Input, Output: *m.Cost.Output}
		if m.Cost.CacheRead != nil {
			e.CacheRead = *m.Cost.CacheRead
		}
		if m.Cost.CacheWrite != nil {
			e.CacheWrite = *m.Cost.CacheWrite
		}
		for _, t := range m.Cost.Tiers {
			if t.Tier == nil || t.Tier.Size == nil {
				continue
			}
			tier := priceTier{Size: *t.Tier.Size, Input: t.Input, Output: t.Output}
			if t.CacheRead != nil {
				tier.CacheRead = *t.CacheRead
			}
			if t.CacheWrite != nil {
				tier.CacheWrite = *t.CacheWrite
			}
			e.Tiers = append(e.Tiers, tier)
		}
		sortTiers(e.Tiers)
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("catalog listed no priced %s models", site.OpenCodeGo)
	}
	return entries, nil
}

var flightChunkRE = regexp.MustCompile(`self\.__next_f\.push\(\[1,\s*("(?:[^"\\]|\\.)*")\]\)`)

// commandCodeModel is one row of the plan page's embedded model payload.
type commandCodeModel struct {
	ID             string   `json:"id"`
	InputCost      *float64 `json:"inputCost"`
	OutputCost     *float64 `json:"outputCost"`
	CacheReadCost  *float64 `json:"cacheReadCost"`
	CacheWriteCost *float64 `json:"cacheWriteCost"`
	Tiers          []struct {
		Context string `json:"context"`
		Rates   struct {
			Input      *float64 `json:"input"`
			Output     *float64 `json:"output"`
			CacheRead  *float64 `json:"cacheRead"`
			CacheWrite *float64 `json:"cacheWrite"`
		} `json:"rates"`
	} `json:"tiers"`
}

var contextBoundRE = regexp.MustCompile(`([0-9.]+)\s*([KkMm])`)

// tierBoundFromLabel turns a band label into the threshold it applies above.
// The page words bands as "≤ 256K" (the standard band) and "> 256K" (the band
// that starts there), so only the "≤" ceiling of the previous band is usable -
// the ">" label carries the same number and would double-count.
func tierBoundFromLabel(label string) (int64, bool) {
	if strings.Contains(label, ">") {
		return 0, false
	}
	m := contextBoundRE.FindStringSubmatch(label)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	if strings.EqualFold(m[2], "m") {
		n *= 1024 * 1024
	} else {
		n *= 1024
	}
	return int64(n), true
}

// FetchCommandCodePrices pulls the per-model price table out of the plan page.
//
// The page ships its data as Next.js flight chunks (`self.__next_f.push`), each
// carrying a JSON string; reassembling them yields a plain JSON payload. This
// is a rendered-site scrape and will break if the page's framework changes -
// which is why the embedded table stays as the fallback and why a parse failure
// is an error rather than a silent empty result.
func FetchCommandCodePrices(ctx context.Context, client *http.Client, url string) ([]priceEntry, error) {
	if url == "" {
		url = CommandCodePricesURL
	}
	if client == nil {
		client = &http.Client{Timeout: priceFetchTimeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "routatic-proxy/price-refresh")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch price page: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("price page returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPricePageBytes))
	if err != nil {
		return nil, fmt.Errorf("read price page: %w", err)
	}

	var sb strings.Builder
	for _, m := range flightChunkRE.FindAllSubmatch(body, -1) {
		var piece string
		if err := json.Unmarshal(m[1], &piece); err != nil {
			continue // a chunk that is not a JSON string is not ours to read
		}
		sb.WriteString(piece)
	}
	// The flight protocol emits `"$undefined"` where a value is absent; that is
	// not valid JSON, and a real null is what the decoder expects.
	payload := strings.ReplaceAll(sb.String(), `"$undefined"`, "null")

	raw, err := extractModelsArray(payload)
	if err != nil {
		return nil, err
	}
	var models []commandCodeModel
	if err := json.Unmarshal(raw, &models); err != nil {
		return nil, fmt.Errorf("parse model payload: %w", err)
	}

	entries := make([]priceEntry, 0, len(models))
	for _, m := range models {
		if m.ID == "" {
			continue
		}
		base := priceEntry{Match: strings.ToLower(lastPathSegment(m.ID))}
		if len(m.Tiers) > 0 {
			r := m.Tiers[0].Rates
			if r.Input == nil || r.Output == nil {
				continue
			}
			base.Input, base.Output = *r.Input, *r.Output
			if r.CacheRead != nil {
				base.CacheRead = *r.CacheRead
			}
			if r.CacheWrite != nil {
				base.CacheWrite = *r.CacheWrite
			}
		} else {
			if m.InputCost == nil || m.OutputCost == nil {
				continue
			}
			base.Input, base.Output = *m.InputCost, *m.OutputCost
			if m.CacheReadCost != nil {
				base.CacheRead = *m.CacheReadCost
			}
			if m.CacheWriteCost != nil {
				base.CacheWrite = *m.CacheWriteCost
			}
		}
		// Bands after the first price the long-context range; a band repeating
		// the base rate is not a band at all.
		for i := 1; i < len(m.Tiers); i++ {
			bound, ok := tierBoundFromLabel(m.Tiers[i-1].Context)
			if !ok {
				continue
			}
			r := m.Tiers[i].Rates
			if r.Input == nil || r.Output == nil {
				continue
			}
			tier := priceTier{Size: bound, Input: *r.Input, Output: *r.Output}
			if r.CacheRead != nil {
				tier.CacheRead = *r.CacheRead
			}
			if r.CacheWrite != nil {
				tier.CacheWrite = *r.CacheWrite
			}
			if tier.Input == base.Input && tier.Output == base.Output {
				continue
			}
			base.Tiers = append(base.Tiers, tier)
		}
		sortTiers(base.Tiers)
		entries = append(entries, base)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("price page listed no priced models")
	}
	return entries, nil
}

// extractModelsArray returns the raw JSON of the first "models" array in the
// payload, matched by bracket depth rather than a regex so a nested array
// cannot truncate it.
func extractModelsArray(payload string) ([]byte, error) {
	key := `"models":[`
	i := strings.Index(payload, key)
	if i < 0 {
		return nil, fmt.Errorf("price page payload has no models array")
	}
	start := i + len(key) - 1
	depth := 0
	for j := start; j < len(payload); j++ {
		switch payload[j] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return []byte(payload[start : j+1]), nil
			}
		}
	}
	return nil, fmt.Errorf("price page payload has an unterminated models array")
}

func lastPathSegment(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func sortTiers(tiers []priceTier) {
	for i := 1; i < len(tiers); i++ {
		for j := i; j > 0 && tiers[j].Size < tiers[j-1].Size; j-- {
			tiers[j], tiers[j-1] = tiers[j-1], tiers[j]
		}
	}
}

// PriceRefreshConfig is the resolved refresh schedule.
type PriceRefreshConfig struct {
	// CatalogPath is the locally synced models.dev catalog the OpenCode Go
	// prices are read from. Empty disables that half of the refresh.
	CatalogPath string
	// CommandCodeURL is the plan page carrying CommandCode's table.
	CommandCodeURL string
	// Interval is how long to wait between refreshes.
	Interval time.Duration
}

// RefreshPrices fetches both platforms' tables and installs whatever it got.
// It returns the per-platform outcome so the caller can log what actually
// changed; a failure for one platform never blocks or clears the other.
func RefreshPrices(ctx context.Context, cfg PriceRefreshConfig, client *http.Client) (map[string][]priceEntry, map[string]error) {
	fetched := map[string][]priceEntry{}
	errs := map[string]error{}

	if goEntries, err := FetchOpenCodeGoPrices(cfg.CatalogPath); err != nil {
		errs[site.OpenCodeGo] = err
	} else {
		fetched[site.OpenCodeGo] = goEntries
	}
	if ccEntries, err := FetchCommandCodePrices(ctx, client, cfg.CommandCodeURL); err != nil {
		errs[site.CommandCode] = err
	} else {
		fetched[site.CommandCode] = ccEntries
	}
	InstallPrices(fetched)
	return fetched, errs
}

// PriceRefreshLoop refreshes on the configured interval until ctx is done. It
// refreshes once immediately, so a restart does not serve stale prices for a
// whole interval.
func PriceRefreshLoop(ctx context.Context, cfg PriceRefreshConfig, client *http.Client, onResult func(map[string]int, map[string]error)) {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultPriceRefreshInterval
	}
	run := func() {
		fetched, errs := RefreshPrices(ctx, cfg, client)
		if onResult == nil {
			return
		}
		counts := map[string]int{}
		for provider, entries := range fetched {
			counts[provider] = len(entries)
		}
		onResult(counts, errs)
	}
	run()
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
