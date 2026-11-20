package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/transformer"
)

func newAnthropicFirstTestHandler(baseURL string, enabled bool, fallback http.Handler) *AnthropicFirstHandler {
	cfg := &config.Config{AnthropicFirst: config.AnthropicFirstConfig{Enabled: enabled, BaseURL: baseURL}}
	return NewAnthropicFirstHandler(config.NewAtomicConfig(cfg, "/tmp/test-config.json"), fallback)
}

func TestAnthropicFirstHealthyPassthrough(t *testing.T) {
	var gotBody, gotBeta, gotAuth, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		gotBeta = r.Header.Get("anthropic-beta")
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Upstream", "anthropic")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "event: message_stop\ndata: {}\n\n")
	}))
	defer upstream.Close()

	var fallbackCalls atomic.Int32
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		fallbackCalls.Add(1)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", strings.NewReader(`{"model":"claude-sonnet-4-6"}`))
	req.Header.Set("Authorization", "Bearer oauth-token")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20,context-management-2025-06-27")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated || rec.Header().Get("X-Upstream") != "anthropic" {
		t.Fatalf("response = %d %v, want Anthropic 201", rec.Code, rec.Header())
	}
	if gotBody != `{"model":"claude-sonnet-4-6"}` || gotAuth != "Bearer oauth-token" {
		t.Fatalf("body/auth not forwarded: body=%q auth=%q", gotBody, gotAuth)
	}
	if gotBeta == "" || gotPath != "/v1/messages?beta=true" {
		t.Fatalf("beta/path not forwarded: beta=%q path=%q", gotBeta, gotPath)
	}
	if fallbackCalls.Load() != 0 {
		t.Fatal("fallback called for healthy Anthropic response")
	}
}

func TestAnthropicFirstFallsBackOnAvailabilityFailures(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, 500, 503, 529} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "120")
				http.Error(w, "unavailable", status)
			}))
			defer upstream.Close()

			var gotBody string
			h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)
				w.Header().Set("X-Upstream", "opencode")
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("request-body")))

			if rec.Code != http.StatusOK || rec.Header().Get("X-Upstream") != "opencode" || gotBody != "request-body" {
				t.Fatalf("fallback response=%d header=%q body=%q", rec.Code, rec.Header().Get("X-Upstream"), gotBody)
			}
		})
	}
}

func TestAnthropicFirstPreservesClientErrors(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Error", "original")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"type":"error","error":{"message":"original"}}`)
			}))
			defer upstream.Close()

			var fallbackCalls atomic.Int32
			h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				fallbackCalls.Add(1)
			}))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}")))

			if rec.Code != status || rec.Header().Get("X-Error") != "original" || !strings.Contains(rec.Body.String(), "original") {
				t.Fatalf("response was not preserved: status=%d headers=%v body=%q", rec.Code, rec.Header(), rec.Body.String())
			}
			if fallbackCalls.Load() != 0 {
				t.Fatal("fallback called for client error")
			}
		})
	}
}

func TestAnthropicFirstDisabledUsesFallback(t *testing.T) {
	var calls atomic.Int32
	h := newAnthropicFirstTestHandler("https://api.anthropic.com", false, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}")))
	if calls.Load() != 1 || rec.Code != http.StatusNoContent {
		t.Fatalf("disabled mode calls=%d status=%d", calls.Load(), rec.Code)
	}
}

func TestAnthropicFirstTransportFailureFallsBack(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := upstream.URL
	upstream.Close()

	var calls atomic.Int32
	h := newAnthropicFirstTestHandler(baseURL, true, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}")))
	if calls.Load() != 1 || rec.Code != http.StatusNoContent {
		t.Fatalf("transport fallback calls=%d status=%d", calls.Load(), rec.Code)
	}
}

func TestAnthropicFirstDoesNotFollowRedirectWithCredentials(t *testing.T) {
	var redirectedCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectedCalls.Add(1)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	h := newAnthropicFirstTestHandler(source.URL, true, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("fallback called for redirect")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}")))
	if rec.Code != http.StatusTemporaryRedirect || redirectedCalls.Load() != 0 {
		t.Fatalf("status=%d redirectedCalls=%d", rec.Code, redirectedCalls.Load())
	}
}

func TestAvailabilityGateBackoffAndSingleProbe(t *testing.T) {
	var gate availabilityGate
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	attempt, ok := gate.allow(now, "https://api.anthropic.com")
	if !ok || attempt.probe {
		t.Fatal("healthy gate should allow a normal request")
	}
	if delay := gate.failed(now, attempt, ""); delay != 30*time.Second {
		t.Fatalf("first backoff=%v, want 30s", delay)
	}
	if _, ok := gate.allow(now.Add(29*time.Second), "https://api.anthropic.com"); ok {
		t.Fatal("request allowed before backoff elapsed")
	}
	probe, ok := gate.allow(now.Add(30*time.Second), "https://api.anthropic.com")
	if !ok || !probe.probe {
		t.Fatal("expected half-open probe")
	}
	if _, ok := gate.allow(now.Add(30*time.Second), "https://api.anthropic.com"); ok {
		t.Fatal("concurrent half-open probe allowed")
	}
	if delay := gate.failed(now.Add(30*time.Second), probe, "120"); delay != 2*time.Minute {
		t.Fatalf("Retry-After delay=%v, want 2m", delay)
	}
	probe, ok = gate.allow(now.Add(150*time.Second), "https://api.anthropic.com")
	if !ok || !probe.probe {
		t.Fatal("expected second half-open probe")
	}
	gate.available(probe)
	if attempt, ok = gate.allow(now.Add(150*time.Second), "https://api.anthropic.com"); !ok || attempt.probe {
		t.Fatal("successful probe did not close gate")
	}
}

func TestAvailabilityGateConcurrentProbe(t *testing.T) {
	var gate availabilityGate
	now := time.Now()
	attempt, _ := gate.allow(now, "upstream")
	gate.failed(now, attempt, "1")

	// One second later the probe window is open.
	probeTime := now.Add(1100 * time.Millisecond)

	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := gate.allow(probeTime, "upstream"); ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 1 {
		t.Fatalf("allowed %d probes, want 1", allowed.Load())
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	delay, ok := parseRetryAfter(now.Add(90*time.Second).Format(http.TimeFormat), now)
	if !ok || delay != 90*time.Second {
		t.Fatalf("delay=%v ok=%v, want 90s", delay, ok)
	}
}

type captureRequestStorage struct {
	mu   sync.Mutex
	recs []history.RequestRecord
}

func (c *captureRequestStorage) InsertRequest(rec history.RequestRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recs = append(c.recs, rec)
	return nil
}

func (c *captureRequestStorage) records() []history.RequestRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]history.RequestRecord(nil), c.recs...)
}

// Traffic served by the direct path used to be invisible: no history row, no
// counters. It now records what this layer can observe, with usage explicitly
// unknown rather than fabricated.
func TestAnthropicFirstRecordsDirectTraffic(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message"}`)
	}))
	defer upstream.Close()

	store := &captureRequestStorage{}
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.NotFoundHandler())
	h.SetAccounting(store, metrics.New())

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","stream":false}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	recs := store.records()
	if len(recs) != 1 {
		t.Fatalf("recorded %d rows, want 1", len(recs))
	}
	got := recs[0]
	if got.Provider != "anthropic" || got.Model != "claude-sonnet-4-6" || !got.Success || !got.UsageUnobserved {
		t.Fatalf("record = %+v, want an unobserved-usage success attributed to anthropic", got)
	}
	if got.ID == "" || rec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("both the row and the response header need an id: row=%q header=%q", got.ID, rec.Header().Get("X-Request-ID"))
	}
	// The row key is per execution. Reusing the caller's correlation id would
	// let INSERT OR REPLACE delete an earlier row.
	if got.ID == rec.Header().Get("X-Request-ID") {
		t.Fatal("the history key must not be the caller-supplied correlation id")
	}
}

func TestAnthropicFirstRowsSurviveAReusedRequestID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"msg_1","type":"message"}`)
	}))
	defer upstream.Close()

	store := &captureRequestStorage{}
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.NotFoundHandler())
	h.SetAccounting(store, metrics.New())

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`))
		req.Header.Set("X-Request-ID", "reused-by-the-client")
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
	if recs := store.records(); len(recs) != 2 {
		t.Fatalf("recorded %d rows, want 2: a reused correlation id must not overwrite history", len(recs))
	}
}

func TestAnthropicFirstRecordsDirectFailures(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
	}))
	defer upstream.Close()

	store := &captureRequestStorage{}
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.NotFoundHandler())
	h.SetAccounting(store, metrics.New())

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	recs := store.records()
	if len(recs) != 1 {
		t.Fatalf("recorded %d rows, want 1", len(recs))
	}
	if recs[0].Success || recs[0].ErrorMsg == "" {
		t.Fatalf("record = %+v, want a failed row with the upstream status", recs[0])
	}
}

// A stream that ends before its declared length is a failed delivery, not a
// success with fewer bytes.
func TestAnthropicFirstRecordsTruncatedStream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "4096")
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
		// Returning with fewer bytes than Content-Length makes the client read
		// fail, which is the truncation this row must record.
	}))
	defer upstream.Close()

	store := &captureRequestStorage{}
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.NotFoundHandler())
	h.SetAccounting(store, metrics.New())

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","stream":true}`))
	h.ServeHTTP(httptest.NewRecorder(), req)

	recs := store.records()
	if len(recs) != 1 {
		t.Fatalf("recorded %d rows, want 1", len(recs))
	}
	if recs[0].Success || recs[0].ErrorMsg == "" {
		t.Fatalf("record = %+v, want a failed row for the truncated stream", recs[0])
	}
}

func TestMetricModelForDirectIsBounded(t *testing.T) {
	for model, want := range map[string]string{
		"claude-opus-4-1":   "claude-opus",
		"claude-sonnet-4-6": "claude-sonnet",
		"claude-haiku-4-5":  "claude-haiku",
		"claude-xyz-random": "claude",
		"gpt-4o":            "unknown",
		"":                  "unknown",
	} {
		if got := metricModelForDirect(model); got != want {
			t.Errorf("metricModelForDirect(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestParseRetryAfterZeroFallsBack(t *testing.T) {
	if _, ok := parseRetryAfter("0", time.Now()); ok {
		t.Fatal("Retry-After: 0 must not disable the backoff")
	}
	if _, ok := parseRetryAfter("-5", time.Now()); ok {
		t.Fatal("a negative Retry-After must not disable the backoff")
	}
	d, ok := parseRetryAfter("60", time.Now())
	if !ok || d != time.Minute {
		t.Fatalf("parseRetryAfter(60) = %v, %v, want 1m", d, ok)
	}
	if d, ok := parseRetryAfter("999999", time.Now()); !ok || d != availabilityBackoffMax {
		t.Fatalf("parseRetryAfter(huge) = %v, %v, want the cap", d, ok)
	}
}

func TestParseRetryAfterPastDateFallsBack(t *testing.T) {
	now := time.Now()
	// http.TimeFormat always says GMT; format the instant in UTC or the local
	// wall-clock digits would parse as a future time.
	past := now.UTC().Add(-time.Hour).Format(http.TimeFormat)
	if _, ok := parseRetryAfter(past, now); ok {
		t.Fatal("a past Retry-After date must not reopen the probe window immediately")
	}
}

// A stalled upstream must still be answerable by the platform path. The idle
// watchdog cancels the upstream context, and the fallback used to inherit that
// already-cancelled context - so every platform attempt failed instantly and
// the caller got a 5xx after waiting out the whole budget.
func TestAnthropicFirstIdleStallFallsBackOnLiveContext(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done() // headers arrive, body never does
	}))
	defer upstream.Close()

	var fallbackCtxErr error
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCtxErr = r.Context().Err()
		w.WriteHeader(http.StatusOK)
	}))
	h.idleTimeout = 80 * time.Millisecond

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`)))

	if fallbackCtxErr != nil {
		t.Fatalf("fallback ran with a dead context: %v", fallbackCtxErr)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want the platform answer", rec.Code)
	}
}

// A response this proxy cannot buffer is not an upstream fault. Cooling the
// whole direct path over one oversized answer would divert every later request
// to the platform for no reason.
func TestAnthropicFirstOversizedResponseDoesNotCoolThePath(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		_, _ = io.WriteString(w, strings.Repeat("x", 4096))
	}))
	defer upstream.Close()

	var fallbackCalls atomic.Int32
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	h.maxResponseBytes = 512

	for i := 0; i < 2; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`)))
	}
	if fallbackCalls.Load() != 2 {
		t.Fatalf("fallback ran %d times, want 2", fallbackCalls.Load())
	}
	if upstreamCalls.Load() != 2 {
		t.Fatalf("upstream called %d times, want 2: an oversized body must not open the backoff", upstreamCalls.Load())
	}
}

// clientGoneWriter cancels the request as it fails, which is what a real
// broken pipe looks like from the handler's side.
type clientGoneWriter struct {
	http.ResponseWriter
	cancel context.CancelFunc
}

func (w *clientGoneWriter) Write([]byte) (int, error) {
	w.cancel()
	return 0, errors.New("write: broken pipe")
}

func (w *clientGoneWriter) Flush() {}

// A caller that leaves during the delivery is not a model failure. The
// streaming path has always classified this; the buffered one did not, so a
// disconnection was scored against the model and polluted the success rate.
func TestAnthropicFirstClientGoneDuringWriteIsNotAModelFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"msg_1"}`)
	}))
	defer upstream.Close()

	store := &captureRequestStorage{}
	met := metrics.New()
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.NotFoundHandler())
	h.SetAccounting(store, met)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`)).WithContext(ctx)
	h.ServeHTTP(&clientGoneWriter{ResponseWriter: httptest.NewRecorder(), cancel: cancel}, req)

	recs := store.records()
	if len(recs) != 1 {
		t.Fatalf("recorded %d rows, want 1", len(recs))
	}
	if recs[0].Success || recs[0].ErrorMsg != errClientGone.Error() {
		t.Fatalf("record = %+v, want a client-gone row", recs[0])
	}
	if got := met.GetSnapshot().RequestsFailed; got != 0 {
		t.Fatalf("failed counter = %d, want 0: the caller left, no model was at fault", got)
	}
}

func TestReadAllWithIdleTooLarge(t *testing.T) {
	_, err := readAllWithIdle(context.Background(), strings.NewReader(strings.Repeat("x", 64)), func() {}, 32)
	if !errors.Is(err, errDirectTooLarge) {
		t.Fatalf("err = %v, want errDirectTooLarge", err)
	}
}

func TestStreamEndErrorClassification(t *testing.T) {
	idle, cancelIdle := context.WithCancelCause(context.Background())
	cancelIdle(transformer.ErrStreamIdle)
	if !errors.Is(streamEndError(idle), transformer.ErrStreamIdle) {
		t.Fatalf("streamEndError(idle) = %v, want ErrStreamIdle", streamEndError(idle))
	}

	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	if got := streamEndError(expired); errors.Is(got, errClientGone) {
		t.Fatalf("a total-timeout expiry was classified as the caller leaving: %v", got)
	}
}

func TestTruncateModelIDCutsOnRuneBoundary(t *testing.T) {
	long := strings.Repeat("模", 200) // 3 bytes each
	got := truncateModelID(long)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateModelID split a rune: %q", got)
	}
	if len(got) > 256 || len(got) < 254 {
		t.Fatalf("len = %d, want the longest valid prefix within 256", len(got))
	}
	if short := truncateModelID("claude-sonnet-4-6"); short != "claude-sonnet-4-6" {
		t.Fatalf("short id changed: %q", short)
	}
}

// The counters are shared with the platform handler, which counts the same
// inbound request when the direct path falls back. Counting at entry therefore
// reported every fallback twice; only the path that served may count.
func TestAnthropicFirstCountsOnlyTheTrafficItServed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	met := metrics.New()
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.SetAccounting(nil, met)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`)))

	if got := met.GetSnapshot().RequestsReceived; got != 0 {
		t.Fatalf("received = %d, want 0: the fallback path counts this request itself", got)
	}

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id":"msg_1"}`)
	}))
	defer healthy.Close()
	direct := newAnthropicFirstTestHandler(healthy.URL, true, http.NotFoundHandler())
	direct.SetAccounting(nil, met)
	direct.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`)))

	if got := met.GetSnapshot().RequestsReceived; got != 1 {
		t.Fatalf("received = %d, want 1 for a request this path served", got)
	}
}

// A buffered answer reaches the client in chunks, each with its own write
// deadline. The single-write version truncated a large response to a slow
// reader, and a chunking bug would only show above the chunk size.
func TestAnthropicFirstDeliversABufferedBodyInChunks(t *testing.T) {
	payload := make([]byte, 3*streamChunkSize+1234)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	h := newAnthropicFirstTestHandler(upstream.URL, true, http.NotFoundHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6"}`)))

	if !bytes.Equal(rec.Body.Bytes(), payload) {
		t.Fatalf("body = %d bytes, want the %d the upstream sent", rec.Body.Len(), len(payload))
	}
}

// A stream that dies mid-delivery is an upstream failure: the next request must
// go to the platform instead of hitting the same dead upstream again. Without
// this, a half-open upstream truncates every response forever.
func TestAnthropicFirstCoolsThePathAfterATruncatedStream(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Content-Length", "4096")
		_, _ = io.WriteString(w, "event: message_start\ndata: {}\n\n")
	}))
	defer upstream.Close()

	var fallbackCalls atomic.Int32
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fallbackCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))

	body := `{"model":"claude-sonnet-4-6","stream":true}`
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))

	if fallbackCalls.Load() != 1 {
		t.Fatalf("fallback ran %d times, want 1", fallbackCalls.Load())
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream called %d times, want 1: a truncated stream must open the backoff", upstreamCalls.Load())
	}
}

// A clean stream is not a failure, so the path stays warm.
func TestAnthropicFirstKeepsThePathAfterAHealthyStream(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {}\n\n")
	}))
	defer upstream.Close()

	h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("fallback called for a healthy stream")
	}))
	body := `{"model":"claude-sonnet-4-6","stream":true}`
	for i := 0; i < 2; i++ {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
	}
	if upstreamCalls.Load() != 2 {
		t.Fatalf("upstream called %d times, want 2", upstreamCalls.Load())
	}
}
