package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/middleware"
	"github.com/routatic/proxy/internal/router"
)

func newRateLimitHandler(t *testing.T, rate int) (*MessagesHandler, *config.AtomicConfig) {
	t.Helper()
	atomic := config.NewAtomicConfig(&config.Config{RateLimitPerMinute: rate}, "")
	h := &MessagesHandler{
		modelRouter:  router.NewModelRouter(atomic),
		rateLimiter:  middleware.NewRateLimiter(100, time.Minute),
		requestIDGen: middleware.NewRequestIDGenerator(),
		metrics:      metrics.New(),
		logger:       slog.Default(),
	}
	return h, atomic
}

func admitStatus(h *MessagesHandler) bool {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	_, ok := h.admitRequest(httptest.NewRecorder(), req)
	return ok
}

// Clearing the setting back to 0 means "the default", and a hot reload has to
// be able to restore it. It used to be skipped whenever it was not positive, so
// an explicit rate stayed in force until the process restarted - and the
// settings form presents 0 as the default.
func TestRateLimitReloadRestoresTheDefault(t *testing.T) {
	h, atomic := newRateLimitHandler(t, 1)

	if !admitStatus(h) {
		t.Fatal("first request rejected")
	}
	if admitStatus(h) {
		t.Fatal("second request admitted under a 1/minute limit")
	}

	// The operator clears the field: 0 means the default, which is 100.
	atomic.ApplyLoaded(&config.Config{RateLimitPerMinute: 0})
	for i := 0; i < 5; i++ {
		if !admitStatus(h) {
			t.Fatalf("request %d rejected after the setting returned to the default", i)
		}
	}
}

// A limiter injected by a caller must survive the setting's initial zero.
func TestRateLimitInjectedLimiterSurvivesTheDefault(t *testing.T) {
	h, _ := newRateLimitHandler(t, 0)
	h.rateLimiter = middleware.NewRateLimiter(1, time.Minute)
	if !admitStatus(h) {
		t.Fatal("first request rejected")
	}
	if admitStatus(h) {
		t.Fatal("the injected 1/minute limiter was replaced by the config default")
	}
}
