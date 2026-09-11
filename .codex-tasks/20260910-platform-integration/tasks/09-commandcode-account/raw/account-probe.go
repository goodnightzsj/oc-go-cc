// A manually invoked, read-only probe. Credentials stay in the remote process.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/config"
)

// Only plan identifiers and timestamps leave the process as strings. Other
// strings (including account identities) are represented by their type.
func fields(v any, field string) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for key, value := range x {
			if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "secret") {
				out[key] = "[redacted]"
			} else {
				out[key] = fields(value, key)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = fields(value, field)
		}
		return out
	case string:
		switch field {
		case "planId", "status", "currentPeriodStart", "currentPeriodEnd", "periodBasis", "resetAt", "currency":
			return x
		default:
			return "<string>"
		}
	default:
		return v
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println(`{"error":"application configuration could not be loaded"}`)
		os.Exit(1)
	}
	u, err := url.Parse(cfg.CommandCode.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host != "api.commandcode.ai" || u.User != nil || u.RawQuery != "" {
		fmt.Println(`{"error":"probe restricted to the configured official CommandCode origin"}`)
		os.Exit(1)
	}
	keys := cfg.CommandCode.EffectiveAPIKeys()
	if len(keys) == 0 {
		fmt.Println(`{"configured":false}`)
		os.Exit(1)
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	failed := false
	for i, key := range keys {
		for _, path := range []string{"/alpha/billing/credits", "/alpha/billing/subscriptions", "/alpha/usage/summary"} {
			result := map[string]any{"key_index": i, "path": path}
			req, _ := http.NewRequest(http.MethodGet, "https://api.commandcode.ai"+path, nil)
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Accept", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				result["error"] = "request failed"
				failed = true
			} else {
				result["status"] = resp.StatusCode
				body, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK || readErr != nil || len(body) > 1<<20 {
					result["error"] = "non-200 status or unreadable response; body not exported"
					failed = true
				} else {
					var data any
					if json.Unmarshal(body, &data) != nil {
						result["error"] = "invalid JSON; body not exported"
						failed = true
					} else {
						result["data"] = fields(data, "")
					}
				}
			}
			_ = json.NewEncoder(os.Stdout).Encode(result)
		}
	}
	if failed {
		os.Exit(1)
	}
}
