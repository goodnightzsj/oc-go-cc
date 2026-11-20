package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/metrics"
)

// The deploy gate decides success from this endpoint alone, so an instance that
// cannot serve has to say so. It used to answer 200 unconditionally.
func TestHealthReportsReadiness(t *testing.T) {
	h := NewHealthHandler(nil, nil, metrics.New(), nil)

	rec := httptest.NewRecorder()
	h.HandleHealth(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("without a readiness check: status = %d, want 200", rec.Code)
	}

	h.SetReadiness(func() (string, bool) { return "no usable credential for cline-pass", true })
	rec = httptest.NewRecorder()
	h.HandleHealth(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded: status = %d, want 503", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"status":"degraded"`) || !strings.Contains(body, "no usable credential") {
		t.Fatalf("degraded body = %s", body)
	}

	h.SetReadiness(func() (string, bool) { return "", false })
	rec = httptest.NewRecorder()
	h.HandleHealth(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthy: status = %d, want 200", rec.Code)
	}
}
