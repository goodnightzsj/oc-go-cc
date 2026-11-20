package client

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Every adapter's non-streaming path read the upstream body with io.ReadAll and
// no limit, so one oversized answer could take the whole process down with it.
func TestReadAllLimitedRefusesOversizedBodies(t *testing.T) {
	ok, err := ReadAllLimited(strings.NewReader("small"))
	if err != nil || string(ok) != "small" {
		t.Fatalf("small body: %q %v", ok, err)
	}
	big := io.LimitReader(repeatByte{}, MaxResponseBytes+1)
	if _, err := ReadAllLimited(big); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("oversized body: %v, want ErrResponseTooLarge", err)
	}
}

// repeatByte yields bytes forever, so the test does not allocate 256 MiB.
type repeatByte struct{}

func (repeatByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// The nine copies of the error-body read had drifted into three behaviours: two
// capped, some uncapped, and several that discarded the read error.
func TestReadAPIErrorBodyKeepsTheReadError(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{"Retry-After": {"5"}},
		Body:       io.NopCloser(io.MultiReader(strings.NewReader(`{"error":"nope"}`), errReader{})),
	}
	apiErr, err := ReadAPIErrorBody(resp, "****abcd")
	if err == nil {
		t.Fatal("read failure was discarded")
	}
	if apiErr.StatusCode != http.StatusBadGateway || apiErr.Body != `{"error":"nope"}` || apiErr.RetryAfter != "5" || apiErr.KeyHint != "****abcd" {
		t.Fatalf("error body lost: %+v", apiErr)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
