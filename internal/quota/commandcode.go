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
	"sync"
	"time"
)

// CommandCodeReport contains only billing fields observed on the official
// Alpha API. Identity, payment details and browser credentials are not retained.
// Credits are USD-denominated usage credits, not cash or local ledger costs.
type CommandCodeReport struct {
	Credits           *CommandCodeCredits      `json:"credits,omitempty"`
	Subscription      *CommandCodeSubscription `json:"subscription,omitempty"`
	Usage             *CommandCodeUsage        `json:"usage,omitempty"`
	CreditsError      string                   `json:"credits_error,omitempty"`
	SubscriptionError string                   `json:"subscription_error,omitempty"`
	UsageError        string                   `json:"usage_error,omitempty"`
}

type CommandCodeCredits struct {
	Credits *struct {
		FreeCredits      *float64 `json:"freeCredits"`
		MonthlyCredits   *float64 `json:"monthlyCredits"`
		PurchasedCredits *float64 `json:"purchasedCredits"`
	} `json:"credits"`
	WindowLimits *struct {
		Limited  *bool              `json:"limited"`
		FiveHour *CommandCodeWindow `json:"fiveHour"`
		Weekly   *CommandCodeWindow `json:"weekly"`
	} `json:"windowLimits"`
}

type CommandCodeWindow struct {
	Used     *float64 `json:"used"`
	Cap      *float64 `json:"cap"`
	Exceeded *bool    `json:"exceeded"`
	ResetAt  *int64   `json:"resetAt"` // Unix milliseconds; zero means no reset time.
}

type CommandCodeSubscription struct {
	PlanID             string `json:"planId"`
	Status             string `json:"status"`
	CurrentPeriodStart string `json:"currentPeriodStart"`
	CurrentPeriodEnd   string `json:"currentPeriodEnd"`
	CancelAtPeriodEnd  *bool  `json:"cancelAtPeriodEnd"`
}

type CommandCodeUsage struct {
	TotalCount     *int64   `json:"totalCount"`
	CompletedCount *int64   `json:"completedCount"`
	FailedCount    *int64   `json:"failedCount"`
	TotalTokensIn  *int64   `json:"totalTokensIn"`
	TotalTokensOut *int64   `json:"totalTokensOut"`
	TotalTokens    *int64   `json:"totalTokens"`
	TotalCredits   *float64 `json:"totalCredits"`
	PeriodBasis    string   `json:"periodBasis"`
}

// CommandCodeBaseURL preserves the configured origin and gateway mount. An
// unrelated proxy path cannot establish where its account API lives.
func CommandCodeBaseURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.ForceQuery {
		return "", errors.New("CommandCode base_url must be an HTTP(S) Provider API URL without credentials, query or fragment")
	}
	path := strings.TrimRight(u.Path, "/")
	for _, suffix := range []string{"/provider/v1/chat/completions", "/provider/v1/messages", "/provider/v1"} {
		if strings.HasSuffix(path, suffix) {
			u.Path = strings.TrimSuffix(path, suffix) + "/alpha"
			return u.String(), nil
		}
	}
	return "", errors.New("cannot derive the CommandCode Alpha account API from base_url; use a /provider/v1 endpoint on the same gateway")
}

// FetchCommandCode keeps successful blocks when another account endpoint fails.
// A successful subscriptions response with data:null means no returned plan.
func FetchCommandCode(parent context.Context, client *http.Client, alphaBase, apiKey string) (*CommandCodeReport, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("no CommandCode API key configured")
	}
	ctx, cancel := context.WithTimeout(parent, RequestTimeout)
	defer cancel()
	report := new(CommandCodeReport)
	var failures [3]error
	var wg sync.WaitGroup
	for i, path := range []string{"/billing/credits", "/billing/subscriptions", "/usage/summary"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch i {
			case 0:
				data := new(CommandCodeCredits)
				failures[i] = fetchCommandCodeJSON(ctx, client, alphaBase+path, apiKey, data)
				if failures[i] == nil {
					failures[i] = data.validate()
				}
				if failures[i] == nil {
					report.Credits = data
				}
			case 1:
				var envelope struct {
					Success *bool                    `json:"success"`
					Data    *CommandCodeSubscription `json:"data"`
				}
				failures[i] = fetchCommandCodeJSON(ctx, client, alphaBase+path, apiKey, &envelope)
				if failures[i] == nil && (envelope.Success == nil || !*envelope.Success) {
					failures[i] = errors.New("subscription response did not report success")
				}
				if failures[i] == nil && envelope.Data != nil {
					failures[i] = envelope.Data.validate()
				}
				if failures[i] == nil {
					report.Subscription = envelope.Data
				}
			case 2:
				data := new(CommandCodeUsage)
				failures[i] = fetchCommandCodeJSON(ctx, client, alphaBase+path, apiKey, data)
				if failures[i] == nil && (data.TotalCount == nil || data.TotalTokensIn == nil || data.TotalTokensOut == nil || data.TotalTokens == nil || data.TotalCredits == nil || data.PeriodBasis == "") {
					failures[i] = errors.New("usage summary is missing required counts, credits or period")
				}
				if failures[i] == nil {
					report.Usage = data
				}
			}
			if failures[i] != nil {
				failures[i] = fmt.Errorf("CommandCode %s: %w", path, failures[i])
			}
		}()
	}
	wg.Wait()
	if failures[0] != nil && failures[1] != nil && failures[2] != nil {
		return nil, errors.Join(failures[:]...)
	}
	for i, target := range []*string{&report.CreditsError, &report.SubscriptionError, &report.UsageError} {
		if failures[i] != nil {
			*target = failures[i].Error()
		}
	}
	return report, nil
}

func (data *CommandCodeCredits) validate() error {
	if data.Credits == nil || data.Credits.FreeCredits == nil || data.Credits.MonthlyCredits == nil || data.Credits.PurchasedCredits == nil {
		return errors.New("credits response is missing required balances")
	}
	if limits := data.WindowLimits; limits != nil {
		if limits.Limited == nil {
			return errors.New("window limits have no limited flag")
		}
		for _, window := range []*CommandCodeWindow{limits.FiveHour, limits.Weekly} {
			if window != nil && (window.Used == nil || window.Cap == nil || *window.Used < 0 || *window.Cap < 0 || (window.ResetAt != nil && *window.ResetAt < 0)) {
				return errors.New("invalid window usage, cap or reset time")
			}
		}
	}
	return nil
}

func (data *CommandCodeSubscription) validate() error {
	if data.PlanID == "" || data.Status == "" {
		return errors.New("subscription is missing its plan or status")
	}
	for _, stamp := range []string{data.CurrentPeriodStart, data.CurrentPeriodEnd} {
		if stamp != "" {
			if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
				return errors.New("subscription has an invalid billing period timestamp")
			}
		}
	}
	return nil
}

func fetchCommandCodeJSON(ctx context.Context, client *http.Client, endpoint, apiKey string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return errors.New("invalid account endpoint")
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
		return fmt.Errorf("account request failed: %s", strings.ReplaceAll(err.Error(), apiKey, "[redacted]"))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return errors.New("could not read account response")
	}
	if len(body) > maxResponseBytes {
		return errors.New("account response exceeds size limit")
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return errors.New("invalid account JSON or field type")
	}
	return nil
}
