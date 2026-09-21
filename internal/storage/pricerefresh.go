package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

	// ClinePassPricesURL is the ClinePass page carrying the Reference pricing
	// table. It is the platform's own published table and the only complete one
	// it offers; models.dev also lists this provider but gets two DeepSeek rows
	// wrong, so it is not a substitute.
	ClinePassPricesURL = "https://docs.cline.bot/getting-started/clinepass.md"

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

// priceRefreshedAt records when each platform's table was last fetched from its
// publisher. Kept beside priceOverrides because the two answer one question
// together: "are these figures live, or the build-time snapshot?"
//
// This exists because a stale table does not look stale. The project's own
// history is the argument: the embedded snapshot was wrong within weeks of
// shipping (OpenCode Go moved its DeepSeek rows, CommandCode repriced four
// models) and the dashboard showed well-formed numbers from retired rates the
// whole time. The count of rules cannot reveal that; only the age can.
var priceRefreshedAt atomic.Pointer[map[string]time.Time]

// priceSeedCounts is the number of rules in each embedded table, so a caller can
// tell how many rows the live refresh is responsible for.
//
// Entries without a Match are not rules: the seed files carry "_comment" header
// objects, and counting those would overstate the table by two on CommandCode
// while the lookup ignores them entirely.
var priceSeedCounts = sync.OnceValue(func() map[string]int {
	tables, err := rateTables()
	if err != nil {
		return map[string]int{}
	}
	out := make(map[string]int, len(tables))
	for name, entries := range tables {
		rules := 0
		for _, e := range entries {
			if e.Match != "" {
				rules++
			}
		}
		out[name] = rules
	}
	return out
})

// PriceTableState describes one platform's price table: how many rules are
// installed, how many the embedded seed carries, and when the live refresh last
// succeeded. A zero RefreshedAt means the seed is in use.
type PriceTableState struct {
	Rules       int       `json:"rules"`
	SeedRules   int       `json:"seed_rules"`
	RefreshedAt time.Time `json:"refreshed_at,omitempty"`
	Live        bool      `json:"live"`
}

// PriceTables reports the installed price table state per platform. Platforms
// with no published table are absent rather than reported as empty, which would
// suggest they publish prices and lost them.
func PriceTables() map[string]PriceTableState {
	out := map[string]PriceTableState{}
	seeds := priceSeedCounts()
	if p := priceOverrides.Load(); p != nil {
		for name, entries := range *p {
			// Count rules, not entries: the installed table is the merged result
			// and also carries the seed files' "_comment" header objects, which
			// price nothing and must not be reported as rules to a reader
			// comparing this figure against the seed.
			rules := 0
			for _, e := range entries {
				if e.Match != "" {
					rules++
				}
			}
			out[name] = PriceTableState{Rules: rules, SeedRules: seeds[name], Live: true}
		}
	}
	if at := priceRefreshedAt.Load(); at != nil {
		for name, when := range *at {
			state := out[name]
			state.RefreshedAt = when
			out[name] = state
		}
	}
	return out
}

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

// mergePriceEntries overlays a freshly fetched table on its embedded seed. The
// fetched table wins for every rule it carries; a rule only the seed has
// survives.
//
// Replacing wholesale is what un-prices real models. The ClinePass and
// CommandCode pages list only the models those pages advertise, while the seeds
// also carry live-roster models sourced elsewhere (cline-pass/deepseek-v4.1-flash
// and cline-pass/glm-5.3-flash come from models.dev because the docs page omits
// them; CommandCode's longcat-2.0:free is no longer on the plans page). A
// dropped rule does not read as an error - it reads as an unknown cost, the
// same answer an unpriced platform gives, so the loss stays invisible until
// someone reads the numbers.
func mergePriceEntries(seed, fetched []priceEntry) []priceEntry {
	if len(seed) == 0 {
		return fetched
	}
	present := make(map[string]bool, len(fetched))
	out := make([]priceEntry, 0, len(seed)+len(fetched))
	for _, e := range fetched {
		if e.Match == "" {
			continue
		}
		present[e.Match] = true
		out = append(out, e)
	}
	for _, e := range seed {
		if e.Match == "" || present[e.Match] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// InstallPrices installs the refreshed tables. Entries for a platform absent
// from the map are left as they are, so a refresh that only reached one
// platform cannot blank the other; within a platform the fetched rules are
// merged onto the seed rather than replacing it, for the reason mergePriceEntries
// documents.
func InstallPrices(fetched map[string][]priceEntry) {
	if len(fetched) == 0 {
		return
	}
	seeds, _ := rateTables()
	current := map[string][]priceEntry{}
	if p := priceOverrides.Load(); p != nil {
		for k, v := range *p {
			current[k] = v
		}
	}
	// Carry forward the previous timestamps, then stamp only the platforms this
	// call actually fetched. A platform absent from `fetched` keeps both its
	// table and its age: it did not refresh, and claiming it did would report a
	// freshness the data does not have.
	when := map[string]time.Time{}
	if at := priceRefreshedAt.Load(); at != nil {
		for k, v := range *at {
			when[k] = v
		}
	}
	now := time.Now().UTC()
	for k, v := range fetched {
		if len(v) > 0 {
			current[k] = mergePriceEntries(seeds[k], v)
			when[k] = now
		}
	}
	priceOverrides.Store(&current)
	priceRefreshedAt.Store(&when)
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
	// prices are read from. Empty reports a refresh error for that platform.
	CatalogPath string
	// RefreshCatalog updates the local cache before it is read. A failure keeps
	// the previous OpenCode table without blocking the CommandCode refresh.
	RefreshCatalog func(context.Context) error
	// CommandCodeURL is the plan page carrying CommandCode's table.
	CommandCodeURL string
	// ClinePassURL is the docs page carrying ClinePass's table.
	ClinePassURL string
	// Interval is how long to wait between refreshes.
	Interval time.Duration
}

// RefreshPrices fetches every platform's table and installs whatever it got.
// It returns the per-platform outcome so the caller can log what actually
// changed; a failure for one platform never blocks or clears another.
func RefreshPrices(ctx context.Context, cfg PriceRefreshConfig, client *http.Client) (map[string][]priceEntry, map[string]error) {
	fetched := map[string][]priceEntry{}
	errs := map[string]error{}

	if cfg.RefreshCatalog != nil {
		if err := cfg.RefreshCatalog(ctx); err != nil {
			errs[site.OpenCodeGo] = err
		}
	}
	if errs[site.OpenCodeGo] == nil {
		if goEntries, err := FetchOpenCodeGoPrices(cfg.CatalogPath); err != nil {
			errs[site.OpenCodeGo] = err
		} else {
			fetched[site.OpenCodeGo] = goEntries
		}
	}
	if ccEntries, err := FetchCommandCodePrices(ctx, client, cfg.CommandCodeURL); err != nil {
		errs[site.CommandCode] = err
	} else {
		fetched[site.CommandCode] = ccEntries
	}
	if clEntries, err := FetchClinePassPrices(ctx, client, cfg.ClinePassURL); err != nil {
		errs[site.ClinePass] = err
	} else {
		fetched[site.ClinePass] = clEntries
	}
	if err := ctx.Err(); err != nil {
		return nil, map[string]error{site.OpenCodeGo: err, site.CommandCode: err, site.ClinePass: err}
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
		if ctx.Err() != nil {
			return
		}
		fetched, errs := RefreshPrices(ctx, cfg, client)
		if onResult == nil || ctx.Err() != nil {
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

// clinePriceRowRE matches one Markdown table row, capturing its cells.
var clinePriceRowRE = regexp.MustCompile(`(?m)^\|(.+)\|\s*$`)

// clinePriceCellsRE pulls the dollar amounts out of a price row, in column
// order. The page escapes the dollar sign in Markdown ("\$1.40") and writes a
// dash for a column the model does not bill, so a missing amount is a dash
// rather than a zero - which is why the count of matches is not the column
// count.
var clinePriceNumberRE = regexp.MustCompile(`\$([0-9]+(?:\.[0-9]+)?)`)

// clinePriceIDRE reads a model id out of the Model ID table's second column.
var clinePriceIDRE = regexp.MustCompile("`(cline-pass/[^`]+)`")

// clinePriceBandRE splits a model label from its context band: "Qwen3.7 Plus
// (> 256K tokens)" is the model plus a band, "GLM-5.3" is the model alone.
var clinePriceBandRE = regexp.MustCompile(`(?i)^(.*?)\s*\((≤|>)\s*([0-9.]+\s*[KkMm])\s*tokens?\)\s*$`)

// clinePriceTagRE strips the footnote the page attaches to the DeepSeek rows
// ("DeepSeek V4 Pro (Off-peak)<sup>1</sup>"). The marker renders as an HTML tag
// in the page source and as a bare digit once the tag is removed, so both are
// cleaned - but only after the label's own "(...)" suffix, to keep a model
// whose name ends in a number from losing it.
var clinePriceTagRE = regexp.MustCompile(`<[^>]+>`)
var clinePriceFootnoteRE = regexp.MustCompile(`\)\s*\d+\s*$`)

// clinePricePeakSuffixRE removes the peak qualifier that distinguishes the two
// rows a peak-priced model gets ("DeepSeek V4 Pro (Off-peak)"), leaving the
// label the id table actually lists. It is applied after the Peak row is
// skipped, so what remains to strip is the Off-peak one.
var clinePricePeakSuffixRE = regexp.MustCompile(`(?i)\s*\((?:off-)?peak\)\s*$`)

// parseClinePassPricePage reads the ClinePass Reference pricing table.
//
// The page carries two tables and the join between them is what makes this
// page a better price source than any mirror: the first maps a display label
// to the platform's own model id, the second prices that label. Reading the id
// straight from the page is why nothing here has to guess how "Kimi K2.7 Code"
// is spelled as an id, and a guessed slug that folded the wrong character would
// quietly price the wrong model.
//
// Two properties of the price table drive the rest:
//
//   - Peak and Off-peak are two rows for one model, and the peak window is
//     unverified. The row this proxy estimates with is Off-peak, because no
//     peak schedule is registered for this platform: a guessed window would
//     price real traffic at the wrong multiplier while looking well-formed. The
//     Peak rows are skipped, not merged into a band.
//   - A context band is a tier ("Qwen3.7 Plus (> 256K tokens)"), which the tier
//     model already represents. The "≤" row is the base and the ">" row is the
//     tier above it.
//
// A row that names a model the id table does not list is a parse failure rather
// than a dropped model: this is the platform's only price source, so a page
// restructure must surface as an error instead of as missing prices.
func parseClinePassPricePage(body []byte) ([]priceEntry, error) {
	ids := map[string]string{}
	idsDone := false
	byMatch := map[string]*priceEntry{}
	var order []string

	for _, row := range clinePriceRowRE.FindAllStringSubmatch(string(body), -1) {
		cells := strings.Split(row[1], "|")
		if len(cells) < 2 {
			continue
		}
		label := strings.TrimSpace(clinePriceFootnoteRE.ReplaceAllString(clinePriceTagRE.ReplaceAllString(cells[0], ""), ")"))
		if label == "" || strings.EqualFold(label, "Model") || strings.HasPrefix(label, "---") {
			continue
		}
		// The id table comes first; it is the only table with a backticked id.
		if id := clinePriceIDRE.FindStringSubmatch(strings.Join(cells[1:], "|")); id != nil {
			if !idsDone {
				ids[strings.ToLower(label)] = id[1]
			}
			continue
		}
		idsDone = true

		if len(cells) < 5 {
			continue
		}
		lowered := strings.ToLower(label)
		if strings.Contains(lowered, "peak") && !strings.Contains(lowered, "off-peak") {
			continue
		}
		label = strings.TrimSpace(clinePricePeakSuffixRE.ReplaceAllString(label, ""))
		base, sign, band := label, "", ""
		if m := clinePriceBandRE.FindStringSubmatch(label); m != nil {
			base, sign, band = strings.TrimSpace(m[1]), m[2], m[3]
		}
		match, ok := ids[strings.ToLower(base)]
		if !ok {
			return nil, fmt.Errorf("price row %q names a model the id table does not list", label)
		}
		amounts := clinePriceNumberRE.FindAllStringSubmatch(strings.Join(cells[1:], "|"), -1)
		if len(amounts) < 2 {
			return nil, fmt.Errorf("price row %q has no readable input/output rate", label)
		}
		rates := make([]float64, 0, len(amounts))
		for _, a := range amounts {
			v, err := strconv.ParseFloat(a[1], 64)
			if err != nil {
				return nil, fmt.Errorf("price row %q has an unreadable amount %q", label, a[1])
			}
			rates = append(rates, v)
		}
		entry, ok := byMatch[match]
		if !ok {
			entry = &priceEntry{Match: match}
			byMatch[match] = entry
			order = append(order, match)
		}
		if sign == ">" {
			// The ">" row is the band above the threshold the "≤" row names, and
			// the tier model keys on that threshold.
			bound, ok := tierBoundFromLabel(band)
			if !ok {
				return nil, fmt.Errorf("price row %q has an unreadable context band %q", label, band)
			}
			tier := priceTier{Size: bound, Input: rates[0], Output: rates[1]}
			if len(rates) > 2 {
				tier.CacheRead = rates[2]
			}
			if len(rates) > 3 {
				tier.CacheWrite = rates[3]
			}
			entry.Tiers = append(entry.Tiers, tier)
			sortTiers(entry.Tiers)
			continue
		}
		entry.Input, entry.Output = rates[0], rates[1]
		if len(rates) > 2 {
			entry.CacheRead = rates[2]
		}
		if len(rates) > 3 {
			entry.CacheWrite = rates[3]
		}
	}
	if len(order) == 0 {
		return nil, errors.New("no price rows found on the ClinePass page")
	}
	out := make([]priceEntry, 0, len(order))
	for _, match := range order {
		out = append(out, *byMatch[match])
	}
	return out, nil
}

// FetchClinePassPrices reads the ClinePass Reference pricing table from the
// platform's own documentation page.
func FetchClinePassPrices(ctx context.Context, client *http.Client, url string) ([]priceEntry, error) {
	if url == "" {
		url = ClinePassPricesURL
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
	return parseClinePassPricePage(body)
}
