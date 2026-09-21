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
