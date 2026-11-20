package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

// MaxResponseBytes bounds an upstream response this proxy reads into memory.
// Every provider adapter's non-streaming path used io.ReadAll with no limit, so
// a misbehaving or hostile upstream could grow the body until the process
// died - taking every in-flight stream with it. The figure matches the direct
// passthrough's buffer budget; ClinePass reassembly is bounded at 64 MiB
// because it collects token-by-token frames.
const MaxResponseBytes = 256 << 20

// ErrResponseTooLarge reports a body over the limit. It is a source failure,
// not a caller mistake: the upstream answered something this proxy could not
// hold.
var ErrResponseTooLarge = errors.New("upstream response too large")

// ReadAllLimited reads r into memory, refusing past any body larger than
// MaxResponseBytes instead of growing until the process is killed.
func ReadAllLimited(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxResponseBytes {
		return nil, fmt.Errorf("%w (over %d bytes)", ErrResponseTooLarge, MaxResponseBytes)
	}
	return body, nil
}

// ReadAPIErrorBody reads a refused response's body into the APIError every
// adapter reports, alongside the read error. One copy rather than the nine that
// had drifted apart: the capped and the uncapped variants, and the ones that
// discarded the read error and so lost the only diagnostic that explained the
// failure. The error is returned rather than folded into Body because callers
// report it differently (wrapped, joined, settled first).
func ReadAPIErrorBody(resp *http.Response, keyHint string) (*APIError, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return &APIError{
		StatusCode: resp.StatusCode,
		Body:       string(body),
		RetryAfter: resp.Header.Get("Retry-After"),
		KeyHint:    keyHint,
	}, err
}

// ReadAPIError is ReadAPIErrorBody for the callers that only report the status.
func ReadAPIError(resp *http.Response, keyHint string) *APIError {
	apiErr, err := ReadAPIErrorBody(resp, keyHint)
	if err != nil && apiErr.Body == "" {
		apiErr.Body = "unreadable error body: " + err.Error()
	}
	return apiErr
}
