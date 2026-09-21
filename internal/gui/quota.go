package gui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/quota"
	"github.com/routatic/proxy/internal/site"
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

// quotaAccount reports one key's quota, never an aggregate account balance.
// Reports and Error are mutually exclusive; CommandCode retains block errors
// inside a partially successful report.
type quotaAccount struct {
	KeyHint     string                   `json:"key_hint"`
	Report      *quota.Report            `json:"report,omitempty"`
	OpenRouter  *quota.OpenRouterKey     `json:"openrouter,omitempty"`
	CommandCode *quota.CommandCodeReport `json:"commandcode,omitempty"`
	ClinePass   *quota.ClinePassReport   `json:"cline_pass,omitempty"`
	Error       string                   `json:"error,omitempty"`

	// Ledger is this instance's own record for the same billing period, set
	// only for platforms whose account API reports one. It exists so the two
	// figures can be read side by side: a request that reached the platform
	// without passing through this proxy appears in the account's count and
	// not here, and that difference is otherwise invisible.
	Ledger *quotaLedger `json:"ledger,omitempty"`
}

// quotaLedger is a local-ledger total for one billing period.
type quotaLedger struct {
	Requests            int64   `json:"requests"`
	KnownRequests       int64   `json:"known_requests"`
	UnknownCostRequests int64   `json:"unknown_cost_requests"`
	CostUSD             float64 `json:"cost_usd"`
}

type quotaLink struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type quotaResponse struct {
	Provider        string                   `json:"provider"`
	Status          string                   `json:"status"`
	Source          string                   `json:"source"`
	Reason          string                   `json:"reason,omitempty"`
	Currency        string                   `json:"currency,omitempty"`
	Links           []quotaLink              `json:"links"`
	Endpoint        string                   `json:"endpoint"`
	Accounts        []quotaAccount           `json:"accounts"`
	ModelLimits     *quota.ModelLimits       `json:"model_limits,omitempty"`
	ModelUsage      []quotaModelUsage        `json:"model_usage"`
	ModelUsageError string                   `json:"model_usage_error,omitempty"`
	FetchedAt       time.Time                `json:"fetched_at"`
	TTLSeconds      int                      `json:"ttl_seconds"`
	Cached          bool                     `json:"cached"`
	Error           string                   `json:"error,omitempty"`
	Credits         *quota.OpenRouterCredits `json:"credits,omitempty"`
	CreditsStatus   string                   `json:"credits_status,omitempty"`
	CreditsError    string                   `json:"credits_error,omitempty"`
	CreditsEndpoint string                   `json:"credits_endpoint,omitempty"`
	BedrockBilling  *quota.BedrockBilling    `json:"bedrock_billing,omitempty"`
}

type quotaCacheEntry struct {
	Identity string
	Response quotaResponse
}

// quotaModelUsage contains local Go ledger costs, not an official account bill.
// UsedUSD is the known subtotal; a missing price makes Percent unknown.
type quotaModelUsage struct {
	Model               string   `json:"model"`
	UsedUSD             float64  `json:"used_usd"`
	AllowanceUSD        float64  `json:"allowance_usd"`
	Percent             *float64 `json:"percent"`
	Requests            int64    `json:"requests"`
	UnknownCostRequests int64    `json:"unknown_cost_requests"`
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
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.atomicCfg == nil || s.atomicCfg.Get() == nil {
		http.Error(w, "proxy config not available", http.StatusServiceUnavailable)
		return
	}
	provider := config.NormalizeProvider(strings.TrimSpace(r.URL.Query().Get("provider")))
	if !config.SupportedProvider(provider) {
		http.Error(w, "unsupported quota provider", http.StatusBadRequest)
		return
	}
	manualBilling := provider == "aws-bedrock" && r.URL.Query().Get("billing_refresh") == "1"
	if (r.Method == http.MethodPost) != manualBilling {
		http.Error(w, "billing refresh requires an explicit POST; other quota queries require GET", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost {
		// A paid query is an action, never a prefetchable GET or cross-site form.
		if err := http.NewCrossOriginProtection().Check(r); err != nil {
			http.Error(w, "cross-origin billing query denied", http.StatusForbidden)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	cfg := s.atomicCfg.Get()
	resp := quotaResponse{
		Provider: provider, Source: "none", Status: "unavailable",
		Accounts: []quotaAccount{}, ModelUsage: []quotaModelUsage{},
		FetchedAt: time.Now().UTC(), TTLSeconds: int(quotaCacheTTL.Seconds()),
	}
	switch provider {
	case "opencode-go":
		resp.Source, resp.Currency = "upstream_api", "USD"
		resp.Links = []quotaLink{{Kind: "billing", URL: "https://opencode.ai/auth"}, {Kind: "docs", URL: "https://opencode.ai/docs/go/"}}
		s.handleGoQuota(w, r, cfg, resp)
		return
	case "openrouter":
		resp.Source, resp.Currency = "official_api", "USD"
		resp.Links = []quotaLink{{Kind: "billing", URL: "https://openrouter.ai/settings/credits"}, {Kind: "usage", URL: "https://openrouter.ai/activity"}, {Kind: "keys", URL: "https://openrouter.ai/settings/keys"}}
		s.handleOpenRouterQuota(w, r, cfg, resp)
		return
	case "opencode-zen":
		resp.Reason = "no_public_account_api"
		resp.Links = []quotaLink{{Kind: "billing", URL: "https://opencode.ai/auth"}, {Kind: "docs", URL: "https://opencode.ai/docs/zen/"}}
	case "commandcode":
		resp.Source, resp.Currency = "official_alpha_api", "USD"
		resp.Links = []quotaLink{{Kind: "usage", URL: "https://commandcode.ai/usage"}, {Kind: "billing", URL: "https://commandcode.ai/billing"}, {Kind: "keys", URL: "https://commandcode.ai/settings/keys"}}
		s.handleCommandCodeQuota(w, r, cfg, resp)
		return
	case "cline-pass":
		resp.Source, resp.Currency = "official_api", ""
		resp.Links = []quotaLink{{Kind: "usage", URL: "https://app.cline.bot/dashboard/subscription?personal=true"}, {Kind: "keys", URL: "https://app.cline.bot"}, {Kind: "docs", URL: "https://docs.cline.bot/getting-started/clinepass"}}
		s.handleClinePassQuota(w, r, cfg, resp)
		return
	case "aws-bedrock":
		resp.Links = []quotaLink{{Kind: "billing", URL: "https://console.aws.amazon.com/billing/home"}, {Kind: "docs", URL: "https://docs.aws.amazon.com/cost-management/latest/userguide/ce-api.html"}}
		s.handleBedrockBilling(w, r, cfg, resp)
		return
	}
	if len(cfg.ProviderAPIKeys(provider)) == 0 {
		resp.Status = "not_configured"
	}
	writeJSON(w, resp)
}

func (s *Server) handleGoQuota(w http.ResponseWriter, r *http.Request, cfg *config.Config, resp quotaResponse) {
	force := r.URL.Query().Get("refresh") == "1"
	keys := goQuotaKeys(cfg.OpenCodeGo.EffectiveAPIKeys(), cfg.EffectiveAPIKeys())

	endpoint, err := quota.UsageURL(cfg.OpenCodeGo.BaseURL)
	if err != nil {
		resp.Status, resp.Error = "error", err.Error()
		writeJSON(w, resp)
		return
	}
	resp.Endpoint = endpoint
	if len(keys) == 0 {
		resp.Status = "not_configured"
		writeJSON(w, resp)
		return
	}

	if cached := s.cachedQuota(resp.Provider, endpoint, keys, force); cached != nil {
		writeJSON(w, *cached)
		return
	}

	resp.Accounts = fetchQuotaAccounts(r.Context(), resp.Provider, endpoint, keys)
	resp.ModelLimits = s.ensureModelLimits(r.Context())
	resp.ModelUsage, err = s.monthlyModelUsage(resp.Accounts, resp.ModelLimits)
	if err != nil {
		resp.ModelUsageError = err.Error()
	}
	resp.Status = quotaStatus(resp)
	resp.FetchedAt = time.Now().UTC()
	s.storeQuota(endpoint, keys, resp)
	writeJSON(w, resp)
}

func (s *Server) handleOpenRouterQuota(w http.ResponseWriter, r *http.Request, cfg *config.Config, resp quotaResponse) {
	// Browsing a platform is not permission to probe it with legacy global keys.
	// Both quota lookups require explicit OpenRouter credentials; inference keeps
	// its existing global-key fallback for configured routing targets.
	keys := goQuotaKeys(cfg.OpenRouter.EffectiveAPIKeys(), nil)
	managementKey := strings.TrimSpace(cfg.OpenRouter.ManagementAPIKey)
	resp.CreditsStatus = "not_configured"
	endpoint, creditsEndpoint, err := quota.OpenRouterURLs(cfg.OpenRouter.BaseURL)
	if err != nil {
		resp.Status, resp.Error = "error", err.Error()
		writeJSON(w, resp)
		return
	}
	resp.Endpoint, resp.CreditsEndpoint = endpoint, creditsEndpoint
	if len(keys) == 0 && managementKey == "" {
		resp.Status = "not_configured"
		writeJSON(w, resp)
		return
	}
	cacheKeys := append(append([]string{}, keys...), managementKey)
	if cached := s.cachedQuota(resp.Provider, endpoint, cacheKeys, r.URL.Query().Get("refresh") == "1"); cached != nil {
		writeJSON(w, *cached)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), quota.RequestTimeout)
	defer cancel()
	var wg sync.WaitGroup
	if managementKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			credits, err := quota.FetchOpenRouterCredits(ctx, nil, creditsEndpoint, managementKey)
			if err != nil {
				resp.CreditsStatus, resp.CreditsError = "error", err.Error()
				return
			}
			resp.Credits, resp.CreditsStatus = credits, "available"
		}()
	}
	resp.Accounts = fetchQuotaAccounts(ctx, resp.Provider, endpoint, keys)
	wg.Wait()
	resp.Status = quotaStatus(resp)
	resp.FetchedAt = time.Now().UTC()
	s.storeQuota(endpoint, cacheKeys, resp)
	writeJSON(w, resp)
}

func (s *Server) handleBedrockBilling(w http.ResponseWriter, r *http.Request, cfg *config.Config, resp quotaResponse) {
	resp.Source, resp.Endpoint = "official_api", quota.BedrockBillingEndpoint
	resp.TTLSeconds = int(quota.BedrockBillingTTL.Seconds())
	billing := cfg.AWSBedrock.Billing
	if !billing.Enabled {
		resp.Status, resp.Reason = "not_configured", "aws_billing_disabled"
		writeJSON(w, resp)
		return
	}
	if err := billing.Validate(); err != nil {
		resp.Status, resp.Error = "error", err.Error()
		writeJSON(w, resp)
		return
	}
	// Legacy refresh=1 is sent on platform changes and by older clients. Only
	// this separate POST action may initiate a paid Cost Explorer query.
	manual := r.Method == http.MethodPost
	began := time.Now().UTC()
	identity := []string{billing.Profile, billing.LinkedAccountID, began.Format(time.DateOnly)}
	s.bedrockBillingMu.Lock()
	defer s.bedrockBillingMu.Unlock()
	if cached := s.cachedQuota(resp.Provider, resp.Endpoint, identity, false); cached != nil && (!manual || cached.FetchedAt.After(began)) {
		writeJSON(w, *cached)
		return
	}
	if !manual {
		resp.Status, resp.Reason = "unavailable", "aws_billing_refresh_required"
		writeJSON(w, resp)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), quota.RequestTimeout)
	defer cancel()
	fetch := s.fetchBedrockBilling
	if fetch == nil {
		fetch = quota.FetchBedrockBilling
	}
	report, err := fetch(ctx, billing)
	if err != nil {
		resp.Status, resp.Error = "error", err.Error()
	} else if report == nil {
		resp.Status, resp.Error = "error", "AWS billing returned no report"
	} else {
		resp.BedrockBilling, resp.Currency = report, report.Currency
		resp.Status = "available"
		if report.TotalCost == nil {
			resp.Status, resp.Reason = "unavailable", "aws_billing_no_data"
		}
	}
	resp.FetchedAt = time.Now().UTC()
	s.storeQuota(resp.Endpoint, identity, resp)
	writeJSON(w, resp)
}

func (s *Server) handleCommandCodeQuota(w http.ResponseWriter, r *http.Request, cfg *config.Config, resp quotaResponse) {
	keys := goQuotaKeys(cfg.CommandCode.EffectiveAPIKeys(), nil)
	if len(keys) == 0 {
		resp.Status = "not_configured"
		writeJSON(w, resp)
		return
	}
	endpoint, err := quota.CommandCodeBaseURL(cfg.CommandCode.BaseURL)
	if err != nil {
		resp.Status, resp.Error = "error", err.Error()
		writeJSON(w, resp)
		return
	}
	resp.Endpoint = endpoint
	if cached := s.cachedQuota(resp.Provider, endpoint, keys, r.URL.Query().Get("refresh") == "1"); cached != nil {
		writeJSON(w, *cached)
		return
	}
	resp.Accounts = fetchQuotaAccounts(r.Context(), resp.Provider, endpoint, keys)
	s.attachCommandCodeLedger(resp.Accounts)
	resp.Status = quotaStatus(resp)
	resp.FetchedAt = time.Now().UTC()
	s.storeQuota(endpoint, keys, resp)
	writeJSON(w, resp)
}

// attachCommandCodeLedger pairs the account with this instance's own totals for
// the subscription period the account reports, so the dashboard can show the
// official figure and the local one together.
//
// The two sides do not cover the same requests, and the difference is not only
// bypassed traffic: the account summary counts every mode on the account,
// including the vendor CLI, while the per-run list behind it is api-only. So
// this is a reconciliation view, not an equality check - an account that runs
// the CLI will always show a gap that says nothing about this proxy.
//
// Like monthlyModelUsage it refuses to attribute traffic when more than one key
// is configured: local rows carry no key identity, so one account's period
// cannot describe a pool. A missing or unparsable period leaves Ledger nil
// rather than guessing a window, because a wrong window would look like a real
// reconciliation gap.
func (s *Server) attachCommandCodeLedger(accounts []quotaAccount) {
	if s.storage == nil || len(accounts) != 1 {
		return
	}
	report := accounts[0].CommandCode
	if report == nil || report.Subscription == nil {
		return
	}
	start, errStart := time.Parse(time.RFC3339, report.Subscription.CurrentPeriodStart)
	end, errEnd := time.Parse(time.RFC3339, report.Subscription.CurrentPeriodEnd)
	if errStart != nil || errEnd != nil || !end.After(start) {
		return
	}
	win, err := storage.NewAnalytics(s.storage).WindowBetween(start, end)
	if err != nil {
		return
	}
	breakdown, err := storage.NewAnalytics(s.storage).ProviderBreakdown(win)
	if err != nil {
		return
	}
	ledger := &quotaLedger{}
	for _, b := range breakdown {
		if b.Provider == site.CommandCode {
			ledger.Requests = b.Requests
			ledger.KnownRequests = b.KnownRequests
			ledger.UnknownCostRequests = b.UnknownCostRequests
			ledger.CostUSD = b.EstCostUSD
			break
		}
	}
	accounts[0].Ledger = ledger
}

// handleClinePassQuota reports the subscription's three usage windows.
//
// There is no ledger side to this one, unlike CommandCode's: ClinePass is a
// flat monthly plan billed against reference rates, so a local "cost" column
// and an official one would be two estimates of the same consumption rather
// than two independent accounts. The percentage the platform reports is the
// authoritative figure and the only one shown.
func (s *Server) handleClinePassQuota(w http.ResponseWriter, r *http.Request, cfg *config.Config, resp quotaResponse) {
	keys := goQuotaKeys(cfg.ClinePass.EffectiveAPIKeys(), nil)
	if len(keys) == 0 {
		resp.Status = "not_configured"
		writeJSON(w, resp)
		return
	}
	endpoint, err := quota.ClinePassUsageURL(cfg.ClinePass.BaseURL)
	if err != nil {
		resp.Status, resp.Error = "error", err.Error()
		writeJSON(w, resp)
		return
	}
	resp.Endpoint = endpoint
	if cached := s.cachedQuota(resp.Provider, endpoint, keys, r.URL.Query().Get("refresh") == "1"); cached != nil {
		writeJSON(w, *cached)
		return
	}
	resp.Accounts = fetchQuotaAccounts(r.Context(), resp.Provider, endpoint, keys)

	// Attach the absolute ceilings the plan publishes. They are attached after
	// the fetch rather than inside it because they come from a second endpoint
	// and are not part of an account's report: a plan lookup that fails leaves
	// the percentages intact, which is the part the platform computes.
	s.attachClinePassLimits(r.Context(), cfg, resp.Accounts)

	resp.Status = quotaStatus(resp)
	resp.FetchedAt = time.Now().UTC()
	s.storeQuota(endpoint, keys, resp)
	writeJSON(w, resp)
}

// attachClinePassLimits fills each window's LimitUSD from the plan endpoint.
//
// The ceilings are per account rather than per key, so this asks once per
// distinct credential and reuses the answer: every key on the same subscription
// sees the same caps, and a second lookup would only add a round trip. A failure
// is logged and leaves the windows percentage-only - the panel renders that
// case, and losing the supplement must not cost the primary figure.
func (s *Server) attachClinePassLimits(ctx context.Context, cfg *config.Config, accounts []quotaAccount) {
	planURL, err := quota.ClinePassPlanURL(cfg.ClinePass.BaseURL)
	if err != nil {
		slog.Debug("cline-pass plan endpoint unavailable", "err", err)
		return
	}
	client := &http.Client{Timeout: quota.RequestTimeout}
	seen := map[string]bool{}
	for i := range accounts {
		report := accounts[i].ClinePass
		if report == nil || len(report.Windows) == 0 {
			continue
		}
		key := accounts[i].KeyHint
		if seen[key] {
			continue
		}
		seen[key] = true
		// The plaintext key is not on the account, so the lookup runs per
		// account record via the same credential list the fetch used.
		limits, err := s.clinePassLimitsFor(ctx, client, cfg, planURL, i)
		if err != nil {
			slog.Debug("cline-pass plan lookup failed", "err", err)
			continue
		}
		for w := range report.Windows {
			if limit, ok := limits[report.Windows[w].Type]; ok {
				report.Windows[w].LimitUSD = limit
			}
		}
	}
}

// clinePassLimitsFor resolves the credential for one account index and reads
// its plan ceilings.
func (s *Server) clinePassLimitsFor(ctx context.Context, client *http.Client, cfg *config.Config, planURL string, index int) (map[string]float64, error) {
	keys := goQuotaKeys(cfg.ClinePass.EffectiveAPIKeys(), nil)
	if index >= len(keys) {
		return nil, errors.New("no credential for this account")
	}
	return quota.FetchClinePassPlan(ctx, client, planURL, keys[index])
}

func quotaStatus(resp quotaResponse) string {
	available, failed := 0, 0
	for _, account := range resp.Accounts {
		if account.Error != "" {
			failed++
		} else if account.Report != nil || account.OpenRouter != nil {
			available++
		} else if report := account.CommandCode; report != nil {
			available++
			if report.CreditsError != "" || report.SubscriptionError != "" || report.UsageError != "" {
				failed++
			}
		} else if report := account.ClinePass; report != nil {
			available++
			if report.Error != "" {
				failed++
			}
		}
	}
	if resp.Credits != nil {
		available++
	}
	if resp.CreditsError != "" || resp.ModelUsageError != "" {
		failed++
	}
	if available == 0 {
		if failed > 0 {
			return "error"
		}
		return "not_configured"
	}
	if failed > 0 {
		return "partial"
	}
	return "available"
}

// fetchQuotaAccounts queries every key in parallel and keeps the configured
// order so the UI does not reshuffle cards between refreshes.
//
// The lookup is detached from the browser request: the result is cached and
// shared, so a dashboard poll abandoned mid-flight must not fill the cache with
// "context canceled" errors for the rest of the TTL.
func fetchQuotaAccounts(parent context.Context, provider, endpoint string, keys []string) []quotaAccount {
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
			var err error
			switch provider {
			case "openrouter":
				accounts[i].OpenRouter, err = quota.FetchOpenRouterKey(ctx, client, endpoint, key)
			case "commandcode":
				accounts[i].CommandCode, err = quota.FetchCommandCode(ctx, client, endpoint, key)
			case "cline-pass":
				accounts[i].ClinePass, err = quota.FetchClinePass(ctx, client, endpoint, key)
			default:
				accounts[i].Report, err = quota.Fetch(ctx, client, endpoint, key)
			}
			if err != nil {
				accounts[i].Error = err.Error()
			}
		}(i, key)
	}
	wg.Wait()
	return accounts
}

// cachedQuota returns the cached response when it is still fresh and was built
// from the same endpoint and key set; otherwise nil.
func (s *Server) cachedQuota(provider, endpoint string, keys []string, force bool) *quotaResponse {
	if force {
		return nil
	}
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	entry, ok := s.quotaCache[provider]
	if !ok || entry.Identity != quotaCacheKey(endpoint, keys) {
		return nil
	}
	if time.Since(entry.Response.FetchedAt) >= time.Duration(entry.Response.TTLSeconds)*time.Second {
		return nil
	}
	cached := entry.Response
	cached.Cached = true
	return &cached
}

func (s *Server) storeQuota(endpoint string, keys []string, resp quotaResponse) {
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	if s.quotaCache == nil {
		s.quotaCache = make(map[string]quotaCacheEntry)
	}
	s.quotaCache[resp.Provider] = quotaCacheEntry{Identity: quotaCacheKey(endpoint, keys), Response: resp}
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
// SQLite ledger: each model's stored cost within [monthly resets_at − 31d,
// resets_at), priced in the official Go price currency.
//
// The monthly window is a 31-day subscription cycle, not a calendar month:
// the OpenCode ledger's first usage record (2026-08-06T06:56Z) lands minutes
// after the subscription start implied by resets_at (2026-09-06T06:44Z minus
// 31 days = 2026-08-06T06:44Z). Sizing it as 30 days would drop 8/6 from the
// window and undercount every opening day of the cycle.
func (s *Server) monthlyModelUsage(accounts []quotaAccount, limits *quota.ModelLimits) ([]quotaModelUsage, error) {
	// Local records do not carry account/key identity. A first-account window
	// cannot safely be used to attribute traffic across a multi-key pool.
	if limits == nil || s.storage == nil || len(accounts) != 1 {
		return nil, nil
	}
	reset := findMonthlyReset(accounts)
	if reset.IsZero() {
		return nil, nil
	}
	win, err := storage.NewAnalytics(s.storage).WindowBetween(reset.Add(-31*24*time.Hour), reset)
	if err != nil {
		return nil, err
	}
	breakdown, err := storage.NewAnalytics(s.storage).ModelBreakdown(win)
	if err != nil {
		return nil, err
	}
	raw := make(map[string]quotaModelUsage, len(breakdown))
	for _, b := range breakdown {
		if b.Provider != "opencode-go" && b.Provider != "opencode_go" {
			continue
		}
		name := normalizeModelName(b.Model)
		row := raw[name]
		row.UsedUSD += b.EstCostUSD
		row.Requests += b.Requests
		row.UnknownCostRequests += b.UnknownCostRequests
		raw[name] = row
	}
	rows := make([]quotaModelUsage, 0, len(limits.Models))
	for _, m := range limits.Models {
		row, used := raw[normalizeModelName(m.Model)]
		if !used {
			// Only models this instance has actually served show up; the rest
			// of the plan table stays out of the way.
			continue
		}
		row.Model = m.Model
		row.AllowanceUSD = m.AllowanceUSD
		if row.UnknownCostRequests == 0 && m.AllowanceUSD > 0 {
			percent := row.UsedUSD / m.AllowanceUSD * 100
			row.Percent = &percent
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UsedUSD != rows[j].UsedUSD {
			return rows[i].UsedUSD > rows[j].UsedUSD
		}
		return rows[i].Model < rows[j].Model
	})
	return rows, nil
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
