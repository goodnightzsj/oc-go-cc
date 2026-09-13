package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/router"
)

// The proxy fronts several platforms, so a refusal the platform named is the
// client's answer. These pin which failures are forwarded and which stay as the
// gateway's own, because getting it wrong is invisible from the outside: every
// case below used to render as the same opaque 502.
func TestClientErrorForwardsUpstreamRefusal(t *testing.T) {
	upstream := &client.APIError{StatusCode: http.StatusForbidden, Body: `{"error":{"message":"model not in plan"}}`}
	status, message := clientError(http.StatusBadGateway, "all models failed", fmt.Errorf("all models failed (4 attempts): %w", upstream))
	if status != http.StatusForbidden {
		t.Errorf("status = %d, want %d", status, http.StatusForbidden)
	}
	if !strings.Contains(message, "model not in plan") {
		t.Errorf("upstream reason missing from %q", message)
	}
}

// An empty active site is this deployment's own misconfiguration. It is still
// reported as unavailable rather than as the client's fault.
func TestClientErrorReportsEmptyActiveSite(t *testing.T) {
	err := fmt.Errorf("%w: active site %q has no routing target for this request", router.ErrWithinSiteNoTarget, "commandcode")
	status, _ := clientError(http.StatusInternalServerError, "routing failed", err)
	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
}

// A failure with no upstream response behind it keeps the caller's status: there
// is nothing to forward and inventing one would be a guess.
func TestClientErrorKeepsCallerStatusWithoutUpstreamCause(t *testing.T) {
	status, message := clientError(http.StatusBadGateway, "all models failed", errors.New("dial tcp: connection refused"))
	if status != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", status, http.StatusBadGateway)
	}
	if message != "all models failed" {
		t.Errorf("message = %q, want the caller's own", message)
	}
}

func TestTruncateErrorStaysValidUTF8(t *testing.T) {
	// A multi-byte rune straddling the cutoff must not be split in half.
	long := strings.Repeat("é", maxForwardedErrorBody)
	got := truncateError(long)
	if len(got) > maxForwardedErrorBody+len("… (truncated)") {
		t.Errorf("truncated output is %d bytes, over the bound", len(got))
	}
	if !strings.HasSuffix(got, "… (truncated)") {
		t.Errorf("truncation is not marked: %q", got[len(got)-20:])
	}
	for _, r := range got {
		if r == '�' {
			t.Fatal("truncation split a rune")
		}
	}
	if short := "ok"; truncateError(short) != short {
		t.Error("a short error must pass through unchanged")
	}
}
