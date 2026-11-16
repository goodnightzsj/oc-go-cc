package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

const clineRoutingRejectionFixture = `{"error":{"code":"API_REQUEST_ERROR_CODE","message":"Error 403: client-supplied \"providerOptions\" model routing is not supported"}}`

func TestClineRoutingRejectionIsNarrow(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   bool
	}{
		{403, clineRoutingRejectionFixture, true},
		{403, `{"error":{"message":"client-supplied \"providerOptions\" model routing is not supported"}}`, true},
		{401, clineRoutingRejectionFixture, false},
		{429, clineRoutingRejectionFixture, false},
		{403, `{"error":{"code":"API_REQUEST_ERROR_CODE","message":"account has no subscription"}}`, false},
		{403, `{"error":{"message":"providerOptions denied for this account"}}`, false},
		{403, `{"error":`, false},
	} {
		if got := clineRoutingRejected(tc.status, tc.body); got != tc.want {
			t.Errorf("status=%d body=%s: rejected=%v want=%v", tc.status, tc.body, got, tc.want)
		}
	}
}

func TestClineRoutingRejectionPreservesConcurrentQuarantine(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, clineRoutingRejectionFixture)
	})
	result := make(chan error, 1)
	go func() {
		_, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: p.Name(), ModelID: "synthetic"})
		result <- err
	}()
	<-entered
	p.keys.Reserve([]string{"cline-key"}).Settle(&client.APIError{StatusCode: http.StatusUnauthorized})
	blocked := p.keys.blockedUntil("cline-key")
	close(release)
	if err := <-result; err == nil {
		t.Fatal("routing rejection unexpectedly succeeded")
	}
	if p.keys.blockedUntil("cline-key") != blocked || p.keys.states["cline-key"].inFlight != 0 {
		t.Fatal("routing rejection erased another request's quarantine or leaked its reservation")
	}
}

func TestClineRoutingRejectionDoesNotCoolKeys(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmtStream(stream), func(t *testing.T) {
			var used []string
			p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				used = append(used, r.Header.Get("Authorization"))
				var request map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if len(used) <= 2 {
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, clineRoutingRejectionFixture)
					return
				}
				if _, pinned := request["provider"]; pinned {
					t.Error("disabled pin still sent provider routing")
				}
				_, _ = io.WriteString(w, "data: {\"model\":\"synthetic\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			})
			cfg := *p.atomic.Get()
			cfg.ClinePass.APIKey = ""
			cfg.ClinePass.APIKeys = []string{"synthetic-first", "synthetic-second"}
			cfg.ClinePass.ChannelPinEnabled = false
			cfg.ClinePass.ChannelPin = "deepseek"
			p.atomic.ApplyLoaded(&cfg)
			model := config.ModelConfig{Provider: p.Name(), ModelID: "cline-pass/deepseek-v4.1-flash-1017"}
			call := func() error {
				if !stream {
					_, err := p.Execute(context.Background(), clinePassRequest(), model)
					return err
				}
				body, err := p.Stream(context.Background(), clinePassRequest(), model)
				if body != nil {
					_, readErr := io.Copy(io.Discard, body)
					err = errors.Join(err, readErr, body.Close())
				}
				return err
			}
			for range 2 {
				err := call()
				var apiErr *client.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Body != clineRoutingRejectionFixture {
					t.Fatalf("routing refusal lost original status/body: %v", err)
				}
			}
			cfg.ClinePass.ChannelPinEnabled = false
			p.atomic.ApplyLoaded(&cfg)
			if err := call(); err != nil {
				t.Fatalf("unpinned request failed after routing rejections: %v", err)
			}
			if len(used) != 3 {
				t.Fatalf("got %d HTTP attempts, want 3 without hidden retries", len(used))
			}
			for _, key := range used {
				if key != "Bearer synthetic-first" {
					t.Fatal("routing rejection rotated a healthy credential")
				}
			}
		})
	}
}

func TestClineRejectionStillCoolsRejectedCredentials(t *testing.T) {
	for _, status := range []int{401, 402, 403, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":{"message":"account rejected; providerOptions is not the cause"}}`)
			})
			cfg := *p.atomic.Get()
			cfg.ClinePass.APIKey = ""
			cfg.ClinePass.APIKeys = []string{"synthetic-first", "synthetic-second"}
			p.atomic.ApplyLoaded(&cfg)
			for range 2 {
				_, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: p.Name(), ModelID: "synthetic"})
				var apiErr *client.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
					t.Fatalf("account refusal lost: %v", err)
				}
			}
			if p.HasUsableKey(cfg.ClinePass.APIKeys) {
				t.Fatal("account rejections no longer cool keys")
			}
		})
	}
}

func TestClineTruncatedRejectionPreservesMetadata(t *testing.T) {
	for _, status := range []int{401, 429, 503} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("http=%d/stream=%t", status, stream), func(t *testing.T) {
				const partial = `{"error":{"message":"synthetic refusal"`
				p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Length", "1000")
					w.Header().Set("Retry-After", "19")
					w.WriteHeader(status)
					_, _ = io.WriteString(w, partial)
				})
				model := config.ModelConfig{Provider: p.Name(), ModelID: "synthetic"}
				var err error
				if stream {
					_, err = p.Stream(t.Context(), clinePassRequest(), model)
				} else {
					_, err = p.Execute(t.Context(), clinePassRequest(), model)
				}
				var apiErr *client.APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != status || apiErr.RetryAfter != "19" || apiErr.Body != partial || !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("truncated refusal lost HTTP/body/retry/read error: %v", err)
				}
				if p.keys.states["cline-key"].inFlight != 0 {
					t.Fatal("rejection leaked key reservation")
				}
			})
		}
	}
}
