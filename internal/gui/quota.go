package gui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/quota"
	"github.com/routatic/proxy/internal/storage"
)

// quotaCacheTTL bounds how often the undocumented upstream usage endpoint is
// polled. The dashboard refreshes on a much shorter interval, so without this
// every open tab would hammer opencode.ai and earn a 429.
const quotaCacheTTL = 30 * time.Second

// limitsRefreshTTL bounds how often the per-model allowance table is pulled
// from the Go docs page. The table changes rarely (new models, adjusted
// allowances) and is refreshed once a day by limitsLoop; a failed refresh
// keeps the previous snapshot.
const limitsRefreshTTL = 24 * time.Hour

// limitsFetchTimeout is the per-attempt bound for pulling the docs page.
const limitsFetchTimeout = 20 * time.Second

// quotaAccount is the quota picture for one configured API key. Report and Error
// are mutually exclusive.
type quotaAccount struct {
	KeyHint string        `json:"key_hint"`
	Report  *quota.Report `json:"report,omitempty"`
	Error   string        `json:"error,omitempty"`
}

type quotaResponse struct {
	Endpoint    string             `json:"endpoint"`
	Accounts    []quotaAccount     `json:"accounts"`
	ModelLimits *quota.ModelLimits `json:"model_limits,omitempty"`
	ModelUsage  []quotaModelUsage  `json:"model_usage,omitempty"`
	FetchedAt   time.Time          `json:"fetched_at"`
	TTLSeconds  int                `json:"ttl_seconds"`
	Cached      bool               `json:"cached"`
	Error       string             `json:"error,omitempty"`
}

// quotaModelUsage is one row of the console-style monthly usage table: what
// this instance spent on the model in the current plan month, converted to
// shared-$60-pool equivalents (raw cost × 60/allowance — the same per-model
// multiplier the OpenCode ledger applies, e.g. DeepSeek V4 Flash ×2), the
// allowance from the docs, and the pool-equivalent share it represents.
// Pool equivalents sum up across models and are comparable to the official
// window totals.
type quotaModelUsage struct {
	Model        string  `json:"model"`
	UsedUSD      float64 `json:"used_usd"`
	AllowanceUSD float64 `json:"allowance_usd"`
	Percent      float64 `json:"percent"`
}
type quotaModelUsage struct {
	Model        string  `json:"model"`
	UsedUSD      float64 `json:"used_usd"`
	AllowanceUSD float64 `json:"allowance_usd"`
	Percent      float64 `json:"percent"`
}

// maskKeyHint renders a key as a stable, non-reversible label. Only the last
// four characters leave the process — enough to tell two configured keys apart,
// never enough to reuse one.
func maskKeyHint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 4 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}

// goQuotaKeys returns the OpenCode Go key pool, preferring provider-specific
// keys over the global pool (the precedence the proxy itself uses) and dropping
// duplicates so one account is not queried twice.
func goQuotaKeys(providerKeys, globalKeys []string) []string {
	keys := providerKeys
	if len(keys) == 0 {
		keys = globalKeys
	}
	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func (s *Server) handleQuota(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.atomicCfg == nil {
		http.Error(w, "proxy config not available", http.StatusServiceUnavailable)
		return
	}

	force := r.URL.Query().Get("refresh") == "1"
	cfg := s.atomicCfg.Get()
	keys := goQuotaKeys(cfg.OpenCodeGo.EffectiveAPIKeys(), cfg.EffectiveAPIKeys())

	endpoint, err := quota.UsageURL(cfg.OpenCodeGo.BaseURL)
	if err != nil {
		writeJSON(w, quotaResponse{FetchedAt: time.Now().UTC(), TTLSeconds: int(quotaCacheTTL.Seconds()), Error: err.Error()})
		return
	}
	if len(keys) == 0 {
		writeJSON(w, quotaResponse{Endpoint: endpoint, Accounts: []quotaAccount{}, FetchedAt: time.Now().UTC(), TTLSeconds: int(quotaCacheTTL.Seconds())})
		return
	}

	if cached := s.cachedQuota(endpoint, keys, force); cached != nil {
		writeJSON(w, *cached)
		return
	}

	resp := quotaResponse{
		Endpoint:    endpoint,
		Accounts:    fetchQuotaAccounts(r.Context(), endpoint, keys),
		ModelLimits: s.ensureModelLimits(r.Context()),
		FetchedAt:   time.Now().UTC(),
		TTLSeconds:  int(quotaCacheTTL.Seconds()),
	}
	resp.ModelUsage = s.monthlyModelUsage(resp.Accounts, resp.ModelLimits)
	s.storeQuota(endpoint, keys, resp)
	writeJSON(w, resp)
}

// fetchQuotaAccounts queries every key in parallel and keeps the configured
// order so the UI does not reshuffle cards between refreshes.
//
// The lookup is detached from the browser request: the result is cached and
// shared, so a dashboard poll abandoned mid-flight must not fill the cache with
// "context canceled" errors for the rest of the TTL.
func fetchQuotaAccounts(parent context.Context, endpoint string, keys []string) []quotaAccount {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), quota.RequestTimeout)
	defer cancel()

	client := &http.Client{Timeout: quota.RequestTimeout}
	accounts := make([]quotaAccount, len(keys))
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			accounts[i] = quotaAccount{KeyHint: maskKeyHint(key)}
			report, err := quota.Fetch(ctx, client, endpoint, key)
			if err != nil {
				accounts[i].Error = err.Error()
				return
			}
			accounts[i].Report = report
		}(i, key)
	}
	wg.Wait()
	return accounts
}

// cachedQuota returns the cached response when it is still fresh and was built
// from the same endpoint and key set; otherwise nil.
func (s *Server) cachedQuota(endpoint string, keys []string, force bool) *quotaResponse {
	if force {
		return nil
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	if s.quotaCache == nil || s.quotaCacheFor != quotaCacheKey(endpoint, keys) {
		return nil
	}
	if time.Since(s.quotaCache.FetchedAt) >= quotaCacheTTL {
		return nil
	}
	cached := *s.quotaCache
	cached.Cached = true
	return &cached
}

func (s *Server) storeQuota(endpoint string, keys []string, resp quotaResponse) {
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	s.quotaCache = &resp
	s.quotaCacheFor = quotaCacheKey(endpoint, keys)
}

// quotaCacheKey identifies the endpoint and key set a cached response belongs
// to, so editing either in Settings invalidates it immediately. The keys are
// hashed: the identity stays exact without holding a second copy of a secret.
func quotaCacheKey(endpoint string, keys []string) string {
	sum := sha256.Sum256([]byte(endpoint + "\x00" + strings.Join(keys, "\x00")))
	return hex.EncodeToString(sum[:])
}

// ensureModelLimits returns the cached per-model allowance snapshot, fetching
// it when missing or older than limitsRefreshTTL. A failed refresh keeps the
// previous snapshot (possibly nil, which just omits the section).
func (s *Server) ensureModelLimits(ctx context.Context) *quota.ModelLimits {
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	if s.modelLimits != nil && time.Since(s.modelLimits.FetchedAt) < limitsRefreshTTL {
		return s.modelLimits
	}
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), limitsFetchTimeout)
	defer cancel()
	lim, err := quota.FetchModelLimits(fetchCtx, &http.Client{Timeout: limitsFetchTimeout}, s.modelLimitsURL...)
	if err != nil {
		return s.modelLimits
	}
	s.modelLimits = lim
	return lim
}

// limitsLoop refreshes the per-model allowance table once a day; it is
// otherwise identical to the handler-side refresh.
func (s *Server) limitsLoop(ctx context.Context) {
	ticker := time.NewTicker(limitsRefreshTTL)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ensureModelLimits(context.Background())
		}
	}
}

// monthlyModelUsage builds the console-style per-model usage rows for the
// current plan month. The upstream usage endpoint only reports window
// percents, never per-model numbers, so spend comes from this instance's own
// SQLite ledger: each model's stored cost within [monthly resets_at − 30d,
// resets_at), converted to shared-pool equivalents (× 60/allowance).
func (s *Server) monthlyModelUsage(accounts []quotaAccount, limits *quota.ModelLimits) []quotaModelUsage {
	if limits == nil || s.storage == nil {
		return nil
	}
	reset := findMonthlyReset(accounts)
	if reset.IsZero() {
		return nil
	}
	win, err := storage.NewAnalytics(s.storage).WindowBetween(reset.Add(-30*24*time.Hour), reset)
	if err != nil {
		return nil
	}
	breakdown, err := storage.NewAnalytics(s.storage).ModelBreakdown(win)
	if err != nil {
		return nil
	}
	raw := make(map[string]float64, len(breakdown))
	for _, b := range breakdown {
		raw[normalizeModelName(b.Model)] += b.EstCostUSD
	}
	rows := make([]quotaModelUsage, 0, len(limits.Models))
	for _, m := range limits.Models {
		spent, used := raw[normalizeModelName(m.Model)]
		if !used {
			// Only models this instance has actually served show up; the rest
			// of the plan table stays out of the way.
			continue
		}
		weight, percent := 1.0, 0.0
		if m.AllowanceUSD > 0 {
			weight = 60 / m.AllowanceUSD
			percent = spent * weight / 60 * 100
		}
		rows = append(rows, quotaModelUsage{Model: m.Model, UsedUSD: spent * weight, AllowanceUSD: m.AllowanceUSD, Percent: percent})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UsedUSD != rows[j].UsedUSD {
			return rows[i].UsedUSD > rows[j].UsedUSD
		}
		return rows[i].Model < rows[j].Model
	})
	return rows
}

// findMonthlyReset returns the monthly window's next reset from the first
// account that reports one.
func findMonthlyReset(accounts []quotaAccount) time.Time {
	for i := range accounts {
		if accounts[i].Report != nil && accounts[i].Report.Monthly != nil && accounts[i].Report.Monthly.ResetsAt != "" {
			if t, err := time.Parse(time.RFC3339, accounts[i].Report.Monthly.ResetsAt); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// normalizeModelName maps a storage model id onto a docs base name so usage
// can be attributed: "deepseek-v4-flash" ↔ "DeepSeek V4 Flash (Off-Peak)" →
// "DeepSeek V4 Flash" (lowercase alphanumerics with variant suffixes
// stripped).
func normalizeModelName(name string) string {
	if i := strings.IndexByte(name, '('); i >= 0 {
		name = name[:i]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
