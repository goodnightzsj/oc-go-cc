package router

import (
	"errors"
	"net/http"
	"testing"

	"github.com/routatic/proxy/internal/client"
)

// A rejected request is the client's answer, not a reason to try another
// platform. Every model in the chain would refuse it identically, so retrying
// spends quota on a known outcome and buries the real cause.
func TestRequestErrorsAreNotWorthFallingBackFor(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,            // malformed body / schema
		http.StatusUnprocessableEntity,   // semantically invalid
		http.StatusRequestEntityTooLarge, // body too large
		http.StatusUnsupportedMediaType,
	} {
		err := &client.APIError{StatusCode: status, Body: "rejected"}
		if !IsNonRetryableRequestError(err) {
			t.Errorf("status %d should stop the chain, not try the next model", status)
		}
		// Wrapped errors must classify the same way.
		if !IsNonRetryableRequestError(wrap(err)) {
			t.Errorf("status %d lost its classification when wrapped", status)
		}
	}
}

// Everything that describes the attempt rather than the request stays
// retryable: that is what a fallback chain is for.
func TestAttemptFailuresRemainRetryable(t *testing.T) {
	for _, status := range []int{
		http.StatusTooManyRequests,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
	} {
		err := &client.APIError{StatusCode: status, Body: "failed"}
		if IsNonRetryableRequestError(err) {
			t.Errorf("status %d describes the attempt, not the request; it must stay retryable", status)
		}
	}
	if IsNonRetryableRequestError(errors.New("connection reset by peer")) {
		t.Error("a network failure is about the attempt and must stay retryable")
	}
	if IsNonRetryableRequestError(nil) {
		t.Error("nil is not a request error")
	}
}

func wrap(err error) error { return &wrapped{err: err} }

type wrapped struct{ err error }

func (w *wrapped) Error() string { return "stage: " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }
