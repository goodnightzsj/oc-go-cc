package transformer

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrStreamReadCanceled is returned by ctxReader.Read when its context is canceled
// or its deadline expires.
var ErrStreamReadCanceled = errors.New("stream read canceled by context")

// ctxReader wraps an io.Reader and aborts Read when ctx is done.
//
// http.Client.Timeout is checked only at request start; once headers arrive, the
// body is streamed and the net/http transport does not enforce any further deadline.
// Without this wrapper, a slow upstream mid-stream can stall a streaming proxy
// forever. The wrapper does not preempt a read that is already blocked in the
// transport — it surfaces a context-canceled error on the next call.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

// NewCtxReader wraps r so that its next Read returns ErrStreamReadCanceled when
// ctx is canceled or its deadline expires.
func NewCtxReader(ctx context.Context, r io.Reader) io.Reader {
	if r == nil {
		return nil
	}
	return &ctxReader{ctx: ctx, r: r}
}

// NewCtxReadCloser is like NewCtxReader but preserves the io.Closer on the
// returned value. If rc is nil, returns nil.
func NewCtxReadCloser(ctx context.Context, rc io.ReadCloser) io.ReadCloser {
	if rc == nil {
		return nil
	}
	return &ctxReadCloser{
		ctxReader: ctxReader{ctx: ctx, r: rc},
		closer:    rc,
	}
}

type ctxReadCloser struct {
	ctxReader
	closer io.Closer
}

func (c *ctxReadCloser) Close() error {
	return c.closer.Close()
}

// UpstreamChannel forwards the wrapped body's channel report, when it has one.
//
// Embedding ctxReader promotes Read and nothing else, so without this the
// wrapper erases UpstreamChannel and the streaming path - which reads the
// channel from exactly this value - would always record an empty channel.
//
// The type assertion is on the interface, not on a concrete provider type:
// transformer must not learn about providers, and a body without the method
// simply reports no channel.
func (c *ctxReadCloser) UpstreamChannel() string {
	reporter, ok := c.closer.(interface{ UpstreamChannel() string })
	if !ok {
		return ""
	}
	return reporter.UpstreamChannel()
}

// UpstreamModel forwards the wrapped body's model report, when it has one.
//
// It needs the same forwarding as UpstreamChannel and for the same reason:
// embedding promotes Read alone, so a wrapper that forwards one observation
// silently erases the other. The streaming path reads both from this value.
func (c *ctxReadCloser) UpstreamModel() string {
	reporter, ok := c.closer.(interface{ UpstreamModel() string })
	if !ok {
		return ""
	}
	return reporter.UpstreamModel()
}

// ServedModel forwards the model the upstream named in its first chunk. It is
// separate from UpstreamModel on purpose: that one is the routing block's
// canonical slug, which arrives at the END of the stream, while this one is the
// chunk-level model that is knowable before any content is written.
// MarkFirstByte forwards the first-byte stamp. The reader asks for it by
// interface, so a wrapper that does not declare it makes the whole chain look
// like a body that never reports one.
func (c *ctxReadCloser) MarkFirstByte() {
	if recorder, ok := c.closer.(interface{ MarkFirstByte() }); ok {
		recorder.MarkFirstByte()
	}
}

// FirstByteAt forwards the first-byte timestamp, for the same reason as the
// observations above: embedding promotes Read alone.
func (c *ctxReadCloser) FirstByteAt() time.Time {
	reporter, ok := c.closer.(interface{ FirstByteAt() time.Time })
	if !ok {
		return time.Time{}
	}
	return reporter.FirstByteAt()
}

func (c *ctxReadCloser) ServedModel() string {
	reporter, ok := c.closer.(interface{ ServedModel() string })
	if !ok {
		return ""
	}
	return reporter.ServedModel()
}

func (c *ctxReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, ErrStreamReadCanceled
	default:
	}

	n, err := c.r.Read(p)
	if n > 0 {
		// Data still valid even if the deadline fired mid-read; the next
		// Read will surface the cancellation.
		return n, err
	}

	select {
	case <-c.ctx.Done():
		return 0, ErrStreamReadCanceled
	default:
		return n, err
	}
}
