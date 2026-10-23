package quota

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// WorkBuddyReport is a gateway-wide snapshot, not a per-key vendor bill.
// Only read-only panel fields are retained; account identities never leave here.
type WorkBuddyReport struct {
	Overview      *WorkBuddyOverview `json:"overview,omitempty"`
	Usage         *WorkBuddyUsage    `json:"usage,omitempty"`
	OverviewError string             `json:"overview_error,omitempty"`
	UsageError    string             `json:"usage_error,omitempty"`
}

type WorkBuddyOverview struct {
	Total              *int               `json:"total"`
	Healthy            *int               `json:"healthy"`
	Cooling            *int               `json:"cooling"`
	Disabled           *int               `json:"disabled"`
	Paused             *int               `json:"paused"`
	InFlightFull       *int               `json:"in_flight_full"`
	BalanceRefreshSec  *int               `json:"balance_refresh_sec"`
	BalanceRefreshedAt string             `json:"balance_refreshed_at"`
	Accounts           []WorkBuddyAccount `json:"accounts"`
}

type WorkBuddyAccount struct {
	Realm          string `json:"realm"`
	Credits        *int64 `json:"credits"`
	CreditsTotal   *int64 `json:"credits_total"` // -1: unlimited; absent/0: unknown ceiling.
	Cooling        bool   `json:"cooling"`
	Disabled       bool   `json:"disabled"`
	Paused         bool   `json:"paused"`
	ReserveBlocked bool   `json:"reserve_blocked"`
}

// Token totals keep the gateway's gross prompt convention. Cache tokens are a
// breakdown, not an increment. Credits are only meaningful with credit_samples.
type WorkBuddyCounts struct {
	Requests         *int64   `json:"requests"`
	Errors           *int64   `json:"errors"`
	PromptTokens     *int64   `json:"prompt_tokens"`
	CompletionTokens *int64   `json:"completion_tokens"`
	TotalTokens      *int64   `json:"total_tokens"`
	CacheHitTokens   *int64   `json:"cache_hit_tokens"`
	CacheMissTokens  *int64   `json:"cache_miss_tokens"`
	Credits          *float64 `json:"credits"`
	CreditSamples    *int64   `json:"credit_samples"`
	AvgLatencyMs     *float64 `json:"avg_latency_ms"`
	AvgTPS           *float64 `json:"avg_tokens_per_second"`
}

type WorkBuddyModelUsage struct {
	Key string `json:"key"`
	WorkBuddyCounts
}

type WorkBuddyUsagePoint struct {
	Time  string `json:"t"` // Gateway-local calendar hour, deliberately not a browser timestamp.
	Scope string `json:"scope"`
	WorkBuddyCounts
}

type WorkBuddyUsage struct {
	Totals    *WorkBuddyCounts      `json:"totals"`
	ByModel   []WorkBuddyModelUsage `json:"by_model"`
	Series    []WorkBuddyUsagePoint `json:"series"`
	Generated string                `json:"generated"`
	Hours     int                   `json:"hours"`
}

// WorkBuddyPanelURL preserves the configured origin and an explicit /v1 mount.
// It never guesses a different host for an instance's privileged bearer key.
func WorkBuddyPanelURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.ForceQuery {
		return "", errors.New("WorkBuddy base_url must be an HTTP(S) API URL without credentials, query or fragment")
	}
	path := strings.TrimRight(u.Path, "/")
	if path != "" {
		matched := false
		for _, suffix := range []string{"/v1/chat/completions", "/v1"} {
			if strings.HasSuffix(path, suffix) {
				path = strings.TrimSuffix(path, suffix)
				matched = true
				break
			}
		}
		if !matched {
			return "", errors.New("cannot derive WorkBuddy panel API; use the gateway origin or a /v1 endpoint")
		}
	}
	u.Path = path + "/panel/api"
	return u.String(), nil
}

func FetchWorkBuddy(parent context.Context, client *http.Client, panelBase, apiKey string) (*WorkBuddyReport, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("no WorkBuddy instance API key configured")
	}
	ctx, cancel := context.WithTimeout(parent, RequestTimeout)
	defer cancel()
	report := new(WorkBuddyReport)
	var failures [2]error
	var wg sync.WaitGroup
	for i, path := range []string{"/overview", "/usage?hours=24"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i == 0 {
				data := new(WorkBuddyOverview)
				failures[i] = fetchAccountJSON(ctx, client, panelBase+path, apiKey, data)
				if failures[i] == nil {
					failures[i] = data.validate()
				}
				if failures[i] == nil {
					report.Overview = data
				}
			} else {
				data := new(WorkBuddyUsage)
				failures[i] = fetchAccountJSON(ctx, client, panelBase+path, apiKey, data)
				if failures[i] == nil {
					failures[i] = data.validate()
				}
				if failures[i] == nil {
					data.Hours = 24
					report.Usage = data
				}
			}
			if failures[i] != nil {
				failures[i] = fmt.Errorf("WorkBuddy %s: %w", path, failures[i])
			}
		}()
	}
	wg.Wait()
	if failures[0] != nil && failures[1] != nil {
		return nil, errors.Join(failures[:]...)
	}
	if failures[0] != nil {
		report.OverviewError = failures[0].Error()
	}
	if failures[1] != nil {
		report.UsageError = failures[1].Error()
	}
	return report, nil
}

func (v *WorkBuddyOverview) validate() error {
	for _, n := range []*int{v.Total, v.Healthy, v.Cooling, v.Disabled} {
		if n == nil || *n < 0 {
			return errors.New("overview is missing valid pool counts")
		}
	}
	for _, n := range []*int{v.Paused, v.InFlightFull, v.BalanceRefreshSec} {
		if n != nil && *n < 0 {
			return errors.New("overview has a negative count or refresh interval")
		}
	}
	if v.Accounts == nil {
		return errors.New("overview is missing its account list")
	}
	for _, account := range v.Accounts {
		if (account.Credits != nil && *account.Credits < 0) || (account.CreditsTotal != nil && *account.CreditsTotal < -1) {
			return errors.New("overview has invalid credits")
		}
	}
	if v.BalanceRefreshedAt != "" {
		if _, err := time.Parse(time.RFC3339, v.BalanceRefreshedAt); err != nil {
			return errors.New("overview has an invalid balance timestamp")
		}
	}
	return nil
}

func (v *WorkBuddyCounts) validate() error {
	if v == nil {
		return errors.New("usage is missing totals")
	}
	for _, n := range []*int64{v.Requests, v.Errors, v.PromptTokens, v.CompletionTokens, v.TotalTokens} {
		if n == nil || *n < 0 {
			return errors.New("usage is missing valid request or token counts")
		}
	}
	for _, n := range []*int64{v.CacheHitTokens, v.CacheMissTokens, v.CreditSamples} {
		if n != nil && *n < 0 {
			return errors.New("usage has a negative sample or cache count")
		}
	}
	for _, n := range []*float64{v.Credits, v.AvgLatencyMs, v.AvgTPS} {
		if n != nil && *n < 0 {
			return errors.New("usage has a negative credit or timing value")
		}
	}
	if *v.Errors > *v.Requests || (v.CreditSamples != nil && *v.CreditSamples > *v.Requests) {
		return errors.New("usage samples exceed request attempts")
	}
	return nil
}

func (v *WorkBuddyUsage) validate() error {
	if err := v.Totals.validate(); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, v.Generated); err != nil {
		return errors.New("usage has no valid generation timestamp")
	}
	for _, row := range v.ByModel {
		if row.Key == "" {
			return errors.New("usage model is missing its name")
		}
		if err := row.validate(); err != nil {
			return err
		}
	}
	for _, point := range v.Series {
		if _, err := time.Parse("2006-01-02T15", point.Time); err != nil || point.Scope != "hour" {
			return errors.New("usage has an invalid hourly bucket")
		}
		if err := point.validate(); err != nil {
			return err
		}
	}
	return nil
}
