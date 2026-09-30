package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRound2AnthropicProbeCancellationAllowsNewProbe(t *testing.T) {
	var upstreamCalls, fallbackCalls atomic.Int32
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		switch upstreamCalls.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			close(probeStarted)
			select {
			case <-r.Context().Done():
			case <-releaseProbe:
			}
		default:
			_, _ = io.WriteString(w, "healthy")
		}
	}))
	defer upstream.Close()
	defer close(releaseProbe)
	h := newAnthropicFirstTestHandler(upstream.URL, true, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func(ctx context.Context) *http.Request {
		return httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}")).WithContext(ctx)
	}
	h.ServeHTTP(httptest.NewRecorder(), request(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(httptest.NewRecorder(), request(ctx))
	}()
	select {
	case <-probeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("probe never reached synthetic upstream")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled request did not return")
	}
	h.ServeHTTP(httptest.NewRecorder(), request(context.Background()))
	if upstreamCalls.Load() != 3 || fallbackCalls.Load() != 1 {
		t.Fatalf("canceled probe permanently owns gate: upstream=%d fallback=%d", upstreamCalls.Load(), fallbackCalls.Load())
	}
}

func TestAvailabilityGateOldProbeCannotFinishNewGeneration(t *testing.T) {
	var gate availabilityGate
	now := time.Now()
	initial, _ := gate.allow(now, "a")
	gate.failed(now, initial, "0")
	old, _ := gate.allow(now, "a")
	gate.reset("b")
	gate.reset("a")
	initial, _ = gate.allow(now, "a")
	gate.failed(now, initial, "0")
	current, _ := gate.allow(now, "a")
	gate.abandon(old)
	gate.available(old)
	gate.failed(now, old, "120")
	if _, allowed := gate.allow(now, "a"); allowed {
		t.Fatal("old probe released current probe")
	}
	gate.abandon(current)
	if next, allowed := gate.allow(now, "a"); !allowed || !next.probe {
		t.Fatal("current probe did not release neutrally")
	}
}
