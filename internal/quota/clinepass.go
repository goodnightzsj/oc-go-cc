package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ClinePassUsageURL is the ClinePass plan-limits endpoint, relative to the
// configured Chat Completions endpoint's origin.
const clinePassUsagePath = "/api/v1/users/me/plan/usage-limits"

// ClinePassReport is the ClinePass subscription's three rolling windows.
//
// These are percentages of a flat monthly plan, not money: ClinePass bills
// $9.99/month and measures usage against reference rates, so percentUsed is
// the only unit the platform publishes and the one the dashboard shows.
type ClinePassReport struct {
	Windows []ClinePassWindow `json:"windows"`
	Error   string            `json:"error,omitempty"`
}

// ClinePassWindow is one measured window. The platform names three
// (five_hour, weekly, monthly) and may add more, so an unrecognised type is
// carried through rather than dropped - the label is the platform's own.
type ClinePassWindow struct {
	Type        string     `json:"type"`
	PercentUsed float64    `json:"percent_used"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
	// LimitUSD is the window's absolute ceiling in dollars, when the plan
	// endpoint reported one. percentUsed alone says how full the window is but
	// not how much room that represents; the two together let the panel show
	// the remaining amount without replacing the percentage the platform
	// computes. Zero means the plan did not publish a ceiling for this window.
	LimitUSD float64 `json:"limit_usd,omitempty"`
}

// clinePassPlanPath is the subscription endpoint carrying the absolute
// per-window thresholds. It is a separate call from usage-limits because the
// two answer different halves of the same question: usage-limits reports the
// percentage consumed, and this reports what that percentage is a percentage
// of.
const clinePassPlanPath = "/api/v1/users/me/plan"

// ClinePassPlanURL keeps the configured origin and gateway mount, replacing
// only the path below it, on the same terms as ClinePassUsageURL.
func ClinePassPlanURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.ForceQuery {
		return "", errors.New("ClinePass base_url must be an HTTP(S) API URL without credentials, query or fragment")
	}
	if path := strings.TrimRight(u.Path, "/"); path != "/api/v1/chat/completions" {
		return "", errors.New("cannot derive the ClinePass plan API from base_url; use the /api/v1/chat/completions endpoint")
	}
	u.Path = clinePassPlanPath
	return u.String(), nil
}

// clinePassPlan is the response envelope. The thresholds arrive as integers in
// 1e-8 dollars, the same unit the account's usage records use, so they are
// converted once here rather than at each call site.
type clinePassPlan struct {
	Success *bool `json:"success"`
	Data    *struct {
		Plan *struct {
			Entitlements *struct {
				ClinePass *struct {
					Enabled               *bool `json:"enabled"`
					InferenceCapThreshold *struct {
						Last5HoursUSD *int64 `json:"last5HoursUsageCostUSDPerUser"`
						Last7DaysUSD  *int64 `json:"last7daysUsageCostUSDPerUser"`
						Last30DaysUSD *int64 `json:"last30daysUsageCostUSDPerUser"`
					} `json:"inferenceCapThreshold"`
				} `json:"cline_pass"`
			} `json:"entitlements"`
		} `json:"plan"`
	} `json:"data"`
}

// clinePassCostUnit converts the plan's integer thresholds into dollars.
//
// Verified against the account's own usage records: a request the platform
// billed at costUsd=169879 for 288557 prompt / 1158 completion tokens prices
// out at $0.00169880 on the published reference rates, so one unit is 1e-8
// dollars. The same divisor makes the window sums reproduce the reported
// percentages exactly (5.322% -> five_hour 5).
const clinePassCostUnit = 1e8

// clinePassWindowType maps a plan threshold onto the window name usage-limits
// uses, since the two endpoints name the same periods differently.
var clinePassWindowType = map[string]string{
	"last5HoursUsageCostUSDPerUser": "five_hour",
	"last7daysUsageCostUSDPerUser":  "weekly",
	"last30daysUsageCostUSDPerUser": "monthly",
}

// FetchClinePassPlan reads the plan's absolute per-window ceilings.
//
// A missing or disabled entitlement is reported as an error rather than as
// zero ceilings: zero would render as "no allowance left", which is the
// opposite of what an absent threshold means.
func FetchClinePassPlan(parent context.Context, client *http.Client, endpoint, apiKey string) (map[string]float64, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("no ClinePass API key configured")
	}
	ctx, cancel := context.WithTimeout(parent, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("invalid plan endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	bounded := http.Client{Timeout: RequestTimeout}
	if client != nil {
		bounded = *client
		if bounded.Timeout == 0 {
			bounded.Timeout = RequestTimeout
		}
	}
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := bounded.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plan request failed: %s", strings.ReplaceAll(err.Error(), apiKey, "[redacted]"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("could not read plan response")
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("plan response exceeds size limit")
	}
	var payload clinePassPlan
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid plan JSON or field type")
	}
	if payload.Success == nil || !*payload.Success {
		return nil, errors.New("plan response did not report success")
	}
	if payload.Data == nil || payload.Data.Plan == nil || payload.Data.Plan.Entitlements == nil ||
		payload.Data.Plan.Entitlements.ClinePass == nil {
		return nil, errors.New("plan response carries no ClinePass entitlement")
	}
	entitlement := payload.Data.Plan.Entitlements.ClinePass
	if entitlement.Enabled == nil || !*entitlement.Enabled {
		return nil, errors.New("plan response reports the ClinePass entitlement as inactive")
	}
	if entitlement.InferenceCapThreshold == nil {
		return nil, errors.New("plan response carries no inference cap thresholds")
	}
	threshold := entitlement.InferenceCapThreshold
	out := map[string]float64{}
	// The window names are the plan's own; absent fields simply contribute no
	// ceiling, and the panel falls back to percentage-only for those windows.
	for _, pair := range []struct {
		value *int64
		name  string
	}{
		{threshold.Last5HoursUSD, "last5HoursUsageCostUSDPerUser"},
		{threshold.Last7DaysUSD, "last7daysUsageCostUSDPerUser"},
		{threshold.Last30DaysUSD, "last30daysUsageCostUSDPerUser"},
	} {
		if pair.value == nil || *pair.value <= 0 {
			continue
		}
		if window, ok := clinePassWindowType[pair.name]; ok {
			out[window] = float64(*pair.value) / clinePassCostUnit
		}
	}
	if len(out) == 0 {
		return nil, errors.New("plan response carries no usable window ceilings")
	}
	return out, nil
}

// ClinePassUsageURL keeps the configured origin and gateway mount, replacing
// only the path below it. A base URL that is not the Cline API one is rejected
// rather than rewritten, so a key meant for a private mirror is never sent to
// the public host.
func ClinePassUsageURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.ForceQuery {
		return "", errors.New("ClinePass base_url must be an HTTP(S) API URL without credentials, query or fragment")
	}
	if path := strings.TrimRight(u.Path, "/"); path != "/api/v1/chat/completions" {
		return "", errors.New("cannot derive the ClinePass account API from base_url; use the /api/v1/chat/completions endpoint")
	}
	u.Path = clinePassUsagePath
	return u.String(), nil
}

// clinePassLimits is the response envelope. success is a pointer so a missing
// flag is distinguishable from an explicit false.
type clinePassLimits struct {
	Success *bool `json:"success"`
	Data    *struct {
		Limits []struct {
			Type        string   `json:"type"`
			PercentUsed *float64 `json:"percentUsed"`
			ResetsAt    *string  `json:"resetsAt"`
		} `json:"limits"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// FetchClinePass reads the plan's usage windows from the already-derived
// endpoint.
//
// It takes the resolved URL rather than the base URL, matching every sibling
// fetcher (Fetch, FetchOpenRouterKey, FetchCommandCode): the caller derives
// once and caches against the result. Deriving here too meant the derived value
// was passed back in as if it were a base URL, and ClinePassUsageURL - which
// only accepts a /api/v1/chat/completions path - rejected its own output. The
// handler's own derivation had already succeeded, so the panel showed the right
// endpoint next to a credential error that named a different one.
//
// It takes the same API key as inference; no OAuth is involved. The endpoint is
// undocumented, so it is treated as an unpublished contract: a missing field is
// an error rather than a zero, but an unknown window type is carried through
// instead of failing the whole response.
func FetchClinePass(parent context.Context, client *http.Client, endpoint, apiKey string) (*ClinePassReport, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("no ClinePass API key configured")
	}
	ctx, cancel := context.WithTimeout(parent, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("invalid account endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	bounded := http.Client{Timeout: RequestTimeout}
	if client != nil {
		bounded = *client
		if bounded.Timeout == 0 {
			bounded.Timeout = RequestTimeout
		}
	}
	// A redirect can leave this account application's credential boundary.
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := bounded.Do(req)
	if err != nil {
		return nil, fmt.Errorf("account request failed: %s", strings.ReplaceAll(err.Error(), apiKey, "[redacted]"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("could not read account response")
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("account response exceeds size limit")
	}
	var payload clinePassLimits
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid account JSON or field type")
	}
	if payload.Success == nil || !*payload.Success {
		if payload.Error != nil && payload.Error.Message != "" {
			return nil, fmt.Errorf("account request rejected: %s", payload.Error.Message)
		}
		return nil, errors.New("account response did not report success")
	}
	if payload.Data == nil || payload.Data.Limits == nil {
		return nil, errors.New("account response carries no usage limits")
	}
	report := &ClinePassReport{Windows: make([]ClinePassWindow, 0, len(payload.Data.Limits))}
	for _, limit := range payload.Data.Limits {
		kind := strings.TrimSpace(limit.Type)
		if kind == "" {
			return nil, errors.New("account response has a window with no type")
		}
		if limit.PercentUsed == nil {
			return nil, fmt.Errorf("account response has no percentage for the %s window", kind)
		}
		window := ClinePassWindow{Type: kind, PercentUsed: *limit.PercentUsed}
		if limit.ResetsAt != nil && strings.TrimSpace(*limit.ResetsAt) != "" {
			// The endpoint has been observed returning nanosecond precision
			// ("2026-09-08T17:00:44.598174595Z"), which RFC3339 alone rejects.
			stamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*limit.ResetsAt))
			if err != nil {
				return nil, fmt.Errorf("account response has an invalid reset time for the %s window", kind)
			}
			window.ResetsAt = &stamp
		}
		report.Windows = append(report.Windows, window)
	}
	if len(report.Windows) == 0 {
		return nil, errors.New("account response carries no usage windows")
	}
	return report, nil
}
