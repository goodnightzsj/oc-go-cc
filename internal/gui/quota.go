package gui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/quota"
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
	FetchedAt   time.Time          `json:"fetched_at"`
	TTLSeconds  int                `json:"ttl_seconds"`
	Cached      bool               `json:"cached"`
	Error       string             `json:"error,omitempty"`
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
	if s.met != nil {
		annotateUsedModels(resp.ModelLimits, s.met.GetSnapshot().ModelCounts)
	}
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

// annotateUsedModels flags the docs-table entries that match models the proxy
// has served requests for since startup, so the UI can lead with the models
// this instance actually routes to. Names are normalized to lowercase
// alphanumerics with variant suffixes stripped, so the proxy's
// "deepseek-v4-flash" matches the docs' "DeepSeek V4 Flash (Off-Peak)".
func annotateUsedModels(limits *quota.ModelLimits, counts map[string]int64) {
	if limits == nil || len(counts) == 0 {
		return
	}
	used := make(map[string]bool, len(counts))
	for name, n := range counts {
		if n > 0 {
			used[normalizeModelName(name)] = true
		}
	}
	for i := range limits.Models {
		if used[normalizeModelName(limits.Models[i].Model)] {
			limits.Models[i].Used = true
		}
	}
}

func normalizeModelName(name string) string {
	name = strings.ToLower(name)
	if i := strings.IndexByte(name, '('); i >= 0 {
		name = name[:i]
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
