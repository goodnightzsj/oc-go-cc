package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/routatic/proxy/internal/config"
)

func TestChannelDiagnosticsSafeReasons(t *testing.T) {
	const privateDetail = "synthetic-private-response-detail"
	for _, tc := range []struct {
		reason string
		status int
		body   string
		err    error
	}{
		{"timeout", 0, "", fmt.Errorf("transport: %w", context.DeadlineExceeded)},
		{"timeout", 0, "", &net.DNSError{Err: privateDetail, IsTimeout: true}},
		{"cancelled", 0, "", fmt.Errorf("transport: %w", context.Canceled)},
		{"transport_error", 0, "", errors.New(privateDetail)},
		{"account_refused", 401, privateDetail, nil},
		{"account_refused", 429, privateDetail, nil},
		{"upstream_refused", 502, privateDetail, nil},
		{"stream_failed", 200, "event: error\ndata: {\"error\":{\"message\":\"" + privateDetail + "\"}}\n\n", nil},
		{"invalid_metadata", 200, strings.Replace(routingFrame("base"), `"finalProvider":"base"`, `"finalProvider":123`, 1), nil},
		{"missing_channel", 200, routingFrame(""), nil},
		{"no_alternative", 200, routingFrame("base", "base"), nil},
	} {
		for _, pipeline := range probePipelines {
			for _, pinned := range []bool{false, true} {
				if pinned && tc.reason == "no_alternative" {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/pinned=%t/status=%d", tc.reason, pipeline, pinned, tc.status), func(t *testing.T) {
					cfg := &config.Config{ClinePass: config.ClinePassConfig{
						BaseURL: "https://synthetic.invalid/chat/completions", APIKeys: []string{"synthetic-credential"}, ChannelProbeEnabled: true,
					}}
					p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)
					calls := 0
					p.httpClient.Transport = responseContractTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						_, _ = io.Copy(io.Discard, r.Body)
						status, body := tc.status, tc.body
						if pinned && calls == 1 {
							status, body = 200, routingFrame("base", "base", "other")
						} else if tc.err != nil {
							return nil, tc.err
						}
						return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
					})
					p.probeOneModel(t.Context(), cfg, cfg.ClinePass.APIKeys, "synthetic-model", pipeline)
					snapshot := p.ChannelDiagnostics()
					wantCalls := 1
					if pinned {
						wantCalls = 2
					}
					if snapshot.EnforcementVerified || len(snapshot.Results) != 1 || calls != wantCalls {
						t.Fatalf("unexpected diagnostics: %+v calls=%d", snapshot, calls)
					}
					row := snapshot.Results[0]
					if row.Verdict != "unknown" || row.Reason != tc.reason {
						t.Fatalf("diagnostic reason = %+v want=%s", row, tc.reason)
					}
					encoded, err := json.Marshal(snapshot)
					if err != nil || strings.Contains(string(encoded), privateDetail) || strings.Contains(string(encoded), cfg.ClinePass.APIKeys[0]) {
						t.Fatal("diagnostics exposed private response detail or credential")
					}
					for _, state := range p.keys.states {
						if state.inFlight != 0 || state.consecutive != 0 || !state.blockedUntil.IsZero() {
							t.Fatal("diagnostic reason collection changed key health")
						}
					}
				})
			}
		}
	}
}

func TestChannelDiagnosticsReadFailuresKeepHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		status       int
		body         func() io.ReadCloser
	}{
		{"oversized", "response_too_large", 200, func() io.ReadCloser { return io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<16)+1))) }},
		{"truncated", "stream_failed", 200, func() io.ReadCloser { return &terminalReadError{payload: "synthetic-private-body"} }},
		{"refusal-truncated", "stream_failed", 502, func() io.ReadCloser { return &terminalReadError{payload: "synthetic-private-body"} }},
		{"read-timeout", "timeout", 200, func() io.ReadCloser { return io.NopCloser(iotest.ErrReader(context.DeadlineExceeded)) }},
		{"read-cancelled", "cancelled", 200, func() io.ReadCloser { return io.NopCloser(iotest.ErrReader(context.Canceled)) }},
	} {
		for _, pipeline := range probePipelines {
			for _, pinned := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/pinned=%t", tc.name, pipeline, pinned), func(t *testing.T) {
					cfg := &config.Config{ClinePass: config.ClinePassConfig{BaseURL: "https://synthetic.invalid/chat/completions", APIKeys: []string{"synthetic-credential"}, ChannelProbeEnabled: true}}
					p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)
					calls := 0
					p.httpClient.Transport = responseContractTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						_, _ = io.Copy(io.Discard, r.Body)
						if pinned && calls == 1 {
							return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(routingFrame("base", "base", "other")))}, nil
						}
						return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: tc.body()}, nil
					})
					p.probeOneModel(t.Context(), cfg, cfg.ClinePass.APIKeys, "synthetic-model", pipeline)
					snapshot := p.ChannelDiagnostics()
					if len(snapshot.Results) != 1 {
						t.Fatalf("diagnostics = %+v", snapshot)
					}
					row := snapshot.Results[0]
					if row.Verdict != "unknown" || row.Status != tc.status || row.Reason != tc.reason {
						t.Fatalf("read failure lost evidence: %+v; want status=%d reason=%s", row, tc.status, tc.reason)
					}
					wantBaseline, wantTarget := "", ""
					if pinned {
						wantBaseline, wantTarget = "base", "other"
					}
					if row.Baseline != wantBaseline || row.Target != wantTarget || row.Actual != "" || row.CanonicalSlug != "" {
						t.Fatalf("read failure reused successful evidence: %+v", row)
					}
					encoded, err := json.Marshal(snapshot)
					if err != nil || strings.Contains(string(encoded), "synthetic-private-body") || strings.Contains(string(encoded), cfg.ClinePass.APIKeys[0]) {
						t.Fatal("read diagnostics exposed body or credential")
					}
				})
			}
		}
	}
}
