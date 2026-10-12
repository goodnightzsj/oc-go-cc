package client

import (
	"io"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/transformer"
)

// channelBody stands in for a provider body that reports the serving channel.
type channelBody struct {
	io.Reader
	channel string
}

func (c *channelBody) Close() error            { return nil }
func (c *channelBody) UpstreamChannel() string { return c.channel }

// The channel has to survive every wrapper the request path puts around the
// body. It does not survive by default: embedding an io.ReadCloser promotes only
// the methods that interface declares, so each wrapper silently erases
// UpstreamChannel unless it forwards it.
//
// This is not hypothetical - the capture wrapper did erase it, and capture is
// enabled in the configuration used to investigate the channel, so the value
// would have been lost exactly when it was being looked for.
func TestUpstreamChannelSurvivesTheWrapperChain(t *testing.T) {
	const want = "openai-compatible-private"

	cases := []struct {
		name  string
		build func(io.ReadCloser) io.ReadCloser
	}{
		{"bare", func(b io.ReadCloser) io.ReadCloser { return b }},
		{"capture", func(b io.ReadCloser) io.ReadCloser {
			return CaptureBody(b, func([]byte) {})
		}},
		{"context", func(b io.ReadCloser) io.ReadCloser {
			return transformer.NewCtxReadCloser(t.Context(), b)
		}},
		{"capture then context", func(b io.ReadCloser) io.ReadCloser {
			return transformer.NewCtxReadCloser(t.Context(), CaptureBody(b, func([]byte) {}))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := &channelBody{Reader: strings.NewReader("data: [DONE]\n\n"), channel: want}
			wrapped := tc.build(inner)
			defer func() { _ = wrapped.Close() }()

			if got := UpstreamChannelOf(wrapped); got != want {
				t.Errorf("channel = %q, want %q through the %s wrapper", got, want, tc.name)
			}
		})
	}
}

// A body that does not report a channel must yield "", not a panic and not a
// made-up value. Every platform except ClinePass takes this path.
func TestUpstreamChannelIsEmptyForBodiesThatDoNotReportOne(t *testing.T) {
	plain := io.NopCloser(strings.NewReader("data: [DONE]\n\n"))
	if got := UpstreamChannelOf(plain); got != "" {
		t.Errorf("channel = %q, want empty for a body with no reporter", got)
	}
	if got := UpstreamChannelOf(nil); got != "" {
		t.Errorf("channel = %q, want empty for a nil body", got)
	}
}
