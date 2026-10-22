package client

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/transformer"
)

// channelBody stands in for a provider body that reports the serving channel
// and the model the gateway said it ran.
type channelBody struct {
	io.Reader
	channel   string
	model     string
	firstByte time.Time
}

func (c *channelBody) Close() error            { return nil }
func (c *channelBody) UpstreamChannel() string { return c.channel }
func (c *channelBody) UpstreamModel() string   { return c.model }
func (c *channelBody) MarkFirstByte()          { c.firstByte = time.Unix(1700000000, 0) }
func (c *channelBody) FirstByteAt() time.Time  { return c.firstByte }

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
	const wantModel = "vmc/deepseek-v4p1-flash-1017-contributor-fallbacks"

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
			inner := &channelBody{Reader: strings.NewReader("data: [DONE]\n\n"), channel: want, model: wantModel}
			wrapped := tc.build(inner)
			defer func() { _ = wrapped.Close() }()

			if got := UpstreamChannelOf(wrapped); got != want {
				t.Errorf("channel = %q, want %q through the %s wrapper", got, want, tc.name)
			}
			// The model is a second fact on the same provider body and is erased
			// by exactly the same wrappers, so it is asserted through the same
			// chain rather than a separate one that could drift.
			if got := UpstreamModelOf(wrapped); got != wantModel {
				t.Errorf("upstream model = %q, want %q through the %s wrapper", got, wantModel, tc.name)
			}
			// The first-byte timestamp is a third observation on the same body
			// and is erased by the same wrappers. It was missed on the first
			// pass: production had capture on, the test fixture had it off, and
			// every stored row came back empty while this test stayed green.
			if marker, ok := wrapped.(interface{ MarkFirstByte() }); ok {
				marker.MarkFirstByte()
			} else {
				t.Fatalf("the %s wrapper does not expose MarkFirstByte", tc.name)
			}
			if got := FirstByteAtOf(wrapped); got.IsZero() {
				t.Errorf("first byte timestamp lost through the %s wrapper", tc.name)
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
