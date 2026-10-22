package provider

import (
	"bytes"
	"io"
	"testing"
)

// The gateway rewrites some request ids onto a virtual model and runs that
// behind its own fallback chain. The id the client sent then stops describing
// what produced the answer, and `canonicalSlug` is the only place the real one
// is named. Observing it is what lets the dashboard report it without touching
// the response the client already received.
func TestChannelObserverReportsTheUpstreamModel(t *testing.T) {
	stream := "data: {\"provider_metadata\":{\"gateway\":{\"routing\":{" +
		"\"finalProvider\":\"openai-compatible-private\"," +
		"\"canonicalSlug\":\"private/deepseek-v4p1-flash-1017-contributor\"}}}}\n\n" +
		"data: [DONE]\n\n"
	body := newClineChannelBody(io.NopCloser(bytes.NewReader([]byte(stream))), "deepseek", "req-1")
	if _, err := io.ReadAll(body); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := body.UpstreamChannel(); got != "openai-compatible-private" {
		t.Errorf("channel = %q", got)
	}
	if got := body.UpstreamModel(); got != "private/deepseek-v4p1-flash-1017-contributor" {
		t.Errorf("upstream model = %q, want the canonicalSlug", got)
	}
}

// Without the field the observer must report nothing rather than a made-up
// value: a gateway that names only its channel did not rewrite the model.
func TestChannelObserverLeavesUpstreamModelEmptyWhenAbsent(t *testing.T) {
	stream := "data: {\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"deepseek\"}}}}\n\n" +
		"data: [DONE]\n\n"
	body := newClineChannelBody(io.NopCloser(bytes.NewReader([]byte(stream))), "", "req-2")
	_, _ = io.ReadAll(body)
	if got := body.UpstreamModel(); got != "" {
		t.Errorf("upstream model = %q, want empty", got)
	}
}
