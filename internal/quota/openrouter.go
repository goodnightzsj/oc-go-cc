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
)

// OpenRouterKey contains only quota fields, not the account or key identity.
// A nil limit is an uncapped key, not an unlimited account balance. Optional
// usage fields stay null when an upstream does not return them.
type OpenRouterKey struct {
	Limit              *float64 `json:"limit"`
	LimitRemaining     *float64 `json:"limit_remaining"`
	LimitReset         *string  `json:"limit_reset"`
	Usage              float64  `json:"usage"`
	UsageDaily         *float64 `json:"usage_daily"`
	UsageWeekly        *float64 `json:"usage_weekly"`
	UsageMonthly       *float64 `json:"usage_monthly"`
	BYOKUsage          *float64 `json:"byok_usage"`
	BYOKUsageDaily     *float64 `json:"byok_usage_daily"`
	BYOKUsageWeekly    *float64 `json:"byok_usage_weekly"`
	BYOKUsageMonthly   *float64 `json:"byok_usage_monthly"`
	IncludeBYOKInLimit *bool    `json:"include_byok_in_limit"`
	IsFreeTier         *bool    `json:"is_free_tier"`
	ExpiresAt          *string  `json:"expires_at"`
}

// OpenRouterCredits is account-wide and requires a separate management key.
// Remaining credit is TotalCredits - TotalUsage and may be negative.
type OpenRouterCredits struct {
	TotalCredits float64 `json:"total_credits"`
	TotalUsage   float64 `json:"total_usage"`
}

// OpenRouterURLs preserves the configured origin and mount prefix. A malformed
// base must never send a private gateway's credentials to the public service.
func OpenRouterURLs(baseURL string) (string, string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.ForceQuery {
		return "", "", errors.New("OpenRouter base_url must be an HTTP(S) API URL without credentials, query or fragment")
	}
	basePath := strings.TrimRight(u.Path, "/")
	basePath = strings.TrimSuffix(basePath, "/chat/completions")
	if !strings.HasSuffix(basePath, "/v1") {
		return "", "", errors.New("OpenRouter base_url must end in /v1 or /v1/chat/completions")
	}
	u.Path = basePath + "/key"
	keyURL := u.String()
	u.Path = basePath + "/credits"
	return keyURL, u.String(), nil
}

// FetchOpenRouterKey reads the authenticated key's credit cap and UTC usage.
// Contract: https://openrouter.ai/docs/api/reference/limits
func FetchOpenRouterKey(ctx context.Context, client *http.Client, endpoint, apiKey string) (*OpenRouterKey, error) {
	data, err := fetchOpenRouterData(ctx, client, endpoint, apiKey)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"limit", "limit_remaining", "limit_reset", "usage"} {
		if _, ok := data[name]; !ok {
			return nil, fmt.Errorf("OpenRouter key response is missing %s", name)
		}
	}
	if string(data["usage"]) == "null" {
		return nil, errors.New("OpenRouter key response has no usage value")
	}
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var result OpenRouterKey
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("invalid OpenRouter key response: %w", err)
	}
	return &result, nil
}

// FetchOpenRouterCredits only receives the separately configured management key.
// Contract: https://openrouter.ai/docs/api/api-reference/credits/get-credits
func FetchOpenRouterCredits(ctx context.Context, client *http.Client, endpoint, apiKey string) (*OpenRouterCredits, error) {
	data, err := fetchOpenRouterData(ctx, client, endpoint, apiKey)
	if err != nil {
		return nil, err
	}
	var result OpenRouterCredits
	for name, dst := range map[string]*float64{"total_credits": &result.TotalCredits, "total_usage": &result.TotalUsage} {
		value, ok := data[name]
		if !ok || string(value) == "null" {
			return nil, fmt.Errorf("OpenRouter credits response is missing %s", name)
		}
		if err := json.Unmarshal(value, dst); err != nil {
			return nil, fmt.Errorf("invalid OpenRouter %s value: %w", name, err)
		}
	}
	return &result, nil
}

func fetchOpenRouterData(ctx context.Context, client *http.Client, endpoint, apiKey string) (map[string]json.RawMessage, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("no OpenRouter API key configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("invalid OpenRouter quota endpoint")
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
	// Even a same-host redirect may target a different application.
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := bounded.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenRouter quota request failed: %s", strings.ReplaceAll(err.Error(), apiKey, "[redacted]"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Upstreams can echo headers in error bodies, so only status leaves here.
		return nil, fmt.Errorf("OpenRouter quota HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read OpenRouter quota response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("OpenRouter quota response exceeds size limit")
	}
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("invalid OpenRouter quota JSON: %w", err)
	}
	if envelope.Data == nil {
		return nil, errors.New("OpenRouter quota response has no data object")
	}
	return envelope.Data, nil
}
