// Package quota reads the OpenCode Go plan quota windows (5-hour rolling,
// weekly, monthly) from the upstream usage endpoint.
//
//	URL:  <opencode-go-base>/usage  (default https://opencode.ai/zen/go/v1/usage)
//	Auth: Authorization: Bearer <api key>
//
// The endpoint is not part of OpenCode's documented API, so the parser accepts
// every response shape seen in the wild:
//
//	{ "usage": { "rolling": { "status", "percent", "resetsAt" }, ... } }
//	{ "rolling5h": { "usageDollars", "limitDollars", "usagePercent", "resetInSec" }, ... }
//
// When the API reports only a percentage, dollar figures are derived from the
// published plan limits and flagged as derived so the UI can say so.
//
// The parsing rules are ported from ocusage (MIT, https://github.com/muzimu217/ocusage).
package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultUsageURL is the upstream endpoint used when no OpenCode Go base
	// URL is configured.
	DefaultUsageURL = "https://opencode.ai/zen/go/v1/usage"

	// Published OpenCode Go plan limits, used only to derive dollars from a
	// percent-only response.
	Rolling5hLimit = 12.0
	WeeklyLimit    = 30.0
	MonthlyLimit   = 60.0

	// RequestTimeout bounds a single usage lookup.
	RequestTimeout = 15 * time.Second

	maxResponseBytes = 1 << 20
)

// Window is one normalized quota window.
type Window struct {
	Status         string   `json:"status"`
	UsedPercent    float64  `json:"used_percent"`
	UsedDollars    *float64 `json:"used_dollars,omitempty"`
	LimitDollars   *float64 `json:"limit_dollars,omitempty"`
	ResetsAt       string   `json:"resets_at,omitempty"`
	ResetsInSec    *int64   `json:"resets_in_sec,omitempty"`
	HasPercent     bool     `json:"has_percent"`
	PercentDerived bool     `json:"percent_derived"`
}

// Report is the full plan picture for one API key.
type Report struct {
	Plan      string    `json:"plan,omitempty"`
	Rolling5h *Window   `json:"rolling_5h,omitempty"`
	Weekly    *Window   `json:"weekly,omitempty"`
	Monthly   *Window   `json:"monthly,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// UsageURL derives the usage endpoint from a configured OpenCode Go chat
// completions base URL. An unset base URL means the documented default.
//
// A base URL that does not look like the chat completions endpoint is rejected
// rather than silently falling back to opencode.ai: the caller would otherwise
// send an API key intended for a private mirror to the public host.
func UsageURL(baseURL string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return DefaultUsageURL, nil
	}
	if strings.HasSuffix(base, "/usage") {
		return base, nil
	}
	for _, suffix := range []string{"/chat/completions", "/messages", "/responses"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix) + "/usage", nil
		}
	}
	return "", fmt.Errorf("cannot derive the usage endpoint from base_url %q", baseURL)
}

// windowRaw tolerates every field naming seen in the wild so far.
type windowRaw struct {
	Status       string   `json:"status"`
	Percent      *float64 `json:"percent"`
	UsagePercent *float64 `json:"usagePercent"`
	UsedPercent  *float64 `json:"usedPercent"`
	UsageDollars *float64 `json:"usageDollars"`
	UsedDollars  *float64 `json:"usedDollars"`
	LimitDollars *float64 `json:"limitDollars"`
	ResetsAt     string   `json:"resetsAt"`
	ResetAt      string   `json:"resetAt"`
	ResetsInSec  *int64   `json:"resetsInSeconds"`
}

// resetSecKeys covers the naming variants observed across community tools.
// resetInSec vs resetsInSec differ by a single "s", so exact-tag matching alone
// silently drops the field.
var resetSecKeys = []string{"resetsInSeconds", "resetsInSec", "resetInSec", "resets_in_sec", "reset_in_sec"}

func (w *windowRaw) UnmarshalJSON(b []byte) error {
	type alias windowRaw // avoid recursion on the custom unmarshaler
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*w = windowRaw(a)

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for _, k := range resetSecKeys {
		if v, ok := m[k]; ok {
			var n int64
			if err := json.Unmarshal(v, &n); err == nil && n > 0 {
				w.ResetsInSec = &n
				break
			}
		}
	}
	return nil
}

func (w *windowRaw) normalize(planLimit float64) *Window {
	out := &Window{Status: w.Status}
	if out.Status == "" {
		out.Status = "ok"
	}

	percent := w.Percent
	if percent == nil {
		percent = w.UsagePercent
	}
	if percent == nil {
		percent = w.UsedPercent
	}
	if percent != nil {
		out.UsedPercent = *percent
		out.HasPercent = true
	}

	out.UsedDollars = w.UsageDollars
	if out.UsedDollars == nil {
		out.UsedDollars = w.UsedDollars
	}
	out.LimitDollars = w.LimitDollars

	// Percent-only responses: derive dollars from the published plan limits.
	if out.UsedDollars == nil && out.HasPercent {
		used := planLimit * out.UsedPercent / 100
		out.UsedDollars = &used
		out.LimitDollars = &planLimit
		out.PercentDerived = true
	}
	// Dollars without a percentage: the UI needs a fill ratio either way.
	if !out.HasPercent && out.UsedDollars != nil && out.LimitDollars != nil && *out.LimitDollars > 0 {
		out.UsedPercent = *out.UsedDollars / *out.LimitDollars * 100
		out.HasPercent = true
	}

	out.ResetsAt = w.ResetsAt
	if out.ResetsAt == "" {
		out.ResetsAt = w.ResetAt
	}
	out.ResetsInSec = w.ResetsInSec
	return out
}

// Parse walks the JSON defensively: windows may sit at the top level or under
// "usage", and the rolling window may be named rolling5h / rolling / 5h.
func Parse(body []byte) (*Report, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", err)
	}

	src := top
	if u, ok := top["usage"]; ok {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(u, &inner); err == nil {
			src = inner
		}
	}

	rep := &Report{FetchedAt: time.Now().UTC()}
	if p, ok := top["plan"]; ok {
		_ = json.Unmarshal(p, &rep.Plan)
	}

	pick := func(names ...string) (json.RawMessage, bool) {
		for _, n := range names {
			if v, ok := src[n]; ok {
				return v, true
			}
		}
		return nil, false
	}
	decode := func(raw json.RawMessage, planLimit float64) (*Window, error) {
		var wr windowRaw
		if err := json.Unmarshal(raw, &wr); err != nil {
			return nil, err
		}
		return wr.normalize(planLimit), nil
	}

	windows := []struct {
		names []string
		limit float64
		dest  **Window
	}{
		{[]string{"rolling5h", "rolling", "5h", "continuous"}, Rolling5hLimit, &rep.Rolling5h},
		{[]string{"weekly", "week"}, WeeklyLimit, &rep.Weekly},
		{[]string{"monthly", "month"}, MonthlyLimit, &rep.Monthly},
	}
	for _, spec := range windows {
		raw, ok := pick(spec.names...)
		if !ok {
			continue
		}
		w, err := decode(raw, spec.limit)
		if err != nil {
			return nil, fmt.Errorf("%s window: %w", spec.names[0], err)
		}
		*spec.dest = w
	}

	if rep.Rolling5h == nil && rep.Weekly == nil && rep.Monthly == nil {
		keys := make([]string, 0, len(src))
		for k := range src {
			keys = append(keys, k)
		}
		return nil, fmt.Errorf("no usage windows found in response (keys: %s)", strings.Join(keys, ", "))
	}
	return rep, nil
}

// Fetch reads and parses the quota windows for one API key. Upstream failures
// are reported verbatim (including status code) so the UI can show the real
// reason instead of an empty panel.
func Fetch(ctx context.Context, client *http.Client, url, key string) (*Report, error) {
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("no API key")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	if client == nil {
		client = &http.Client{Timeout: RequestTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("HTTP %d: API key rejected — check that the key has a Go subscription", resp.StatusCode)
	case http.StatusNotFound:
		return nil, fmt.Errorf("HTTP 404: no usage endpoint at %s", url)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("HTTP 429: rate limited, retry later")
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}

	rep, err := Parse(body)
	if err != nil {
		return nil, err
	}
	return rep, nil
}
