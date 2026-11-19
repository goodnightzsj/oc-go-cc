package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

// A rejected request must carry the credential label the scheduler stamped on
// the settlement, or the history row cannot say which key was refused.
func TestProviderErrorsCarryKeyHint(t *testing.T) {
	cfg := config.NewAtomicConfig(&config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{APIKey: "synthetic-key-abcd", BaseURL: "https://synthetic.invalid/chat"},
	}, "")
	p := NewOpenCodeGoProvider(cfg, nil)
	p.httpClient = &http.Client{Transport: responseContractTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
			Request:    r,
		}, nil
	})}

	req := &types.MessageRequest{
		Model:     "m",
		MaxTokens: 16,
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
	_, err := p.Execute(context.Background(), req, config.ModelConfig{Provider: "opencode-go", ModelID: "deepseek-v4-flash"})
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want an APIError", err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", apiErr.StatusCode)
	}
	if apiErr.KeyHint == "" {
		t.Fatal("the rejected attempt lost its key hint")
	}
}

func TestRetryAfterIsCapped(t *testing.T) {
	// A huge delta-seconds value used to overflow the scheduler's wait score
	// into a negative number, making a resting key look playable.
	err := &client.APIError{StatusCode: 429, RetryAfter: "3153600000"}
	if got := retryAfter(err, time.Minute); got != time.Hour {
		t.Fatalf("retryAfter = %v, want the one-hour cap", got)
	}
	far := time.Now().Add(100 * 365 * 24 * time.Hour).UTC().Format(http.TimeFormat)
	if got := retryAfter(&client.APIError{StatusCode: 429, RetryAfter: far}, time.Minute); got != time.Hour {
		t.Fatalf("retryAfter(date) = %v, want the one-hour cap", got)
	}
	if got := retryAfter(&client.APIError{StatusCode: 429, RetryAfter: "12"}, time.Minute); got != 12*time.Second {
		t.Fatalf("retryAfter = %v, want 12s", got)
	}
}
