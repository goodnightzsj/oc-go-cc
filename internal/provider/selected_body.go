package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
)

type keyOutcomeContextKey struct{}

// Execute owns parsing through its return; Stream transfers ownership to Close.
// The context carries the same attempt through the providers' protocol helpers.
type keyOutcome struct {
	ctx                 context.Context
	sel                 *selection
	format              core.WireFormat
	stream, requireDone bool
}

func withKeyOutcome(ctx context.Context, format core.WireFormat, stream, requireDone bool) (context.Context, *keyOutcome) {
	o := &keyOutcome{ctx: ctx, format: format, stream: stream, requireDone: requireDone}
	return context.WithValue(ctx, keyOutcomeContextKey{}, o), o
}

func (o *keyOutcome) finish(err error) {
	if errors.Is(err, context.Canceled) && context.Cause(o.ctx) != nil {
		err = context.Cause(o.ctx)
	}
	o.sel.Settle(err)
}

func ownSelectedResponse(ctx context.Context, resp *http.Response, sel *selection) {
	o := ctx.Value(keyOutcomeContextKey{}).(*keyOutcome)
	o.sel = sel.handoff()
	if o.stream {
		resp.Body = &selectedBody{ReadCloser: resp.Body, outcome: o,
			observation: transformer.StreamTermination{Format: o.format, RequireDone: o.requireDone}}
	}
}

// Only observes source bytes; downstream writes never participate in key health.
// Install before capture/model/key-hint wrappers so their metadata stays intact.
type selectedBody struct {
	io.ReadCloser
	outcome     *keyOutcome
	mu          sync.Mutex
	observation transformer.StreamTermination
	closed      bool
	closeOnce   sync.Once
	closeErr    error
}

func (b *selectedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return n, err
	}
	if sourceErr := b.observation.Feed(p[:n]); sourceErr != nil {
		// A later client cancellation cannot erase an already observed source error.
		b.outcome.finish(sourceErr)
	}
	// Native consumers stop at a complete terminal, before HTTP cleanup.
	native := b.observation.Format == core.WireFormatAnthropic || b.observation.Format == core.WireFormatOpenAIResponses
	if err != nil && native && b.observation.Complete() && b.observation.Finish() == nil {
		b.outcome.finish(nil)
	} else if err == io.EOF {
		b.outcome.finish(b.observation.Finish())
	} else if err != nil {
		outcomeErr := err
		if errors.Is(err, context.Canceled) && b.outcome.ctx.Err() == nil {
			outcomeErr = transformer.ErrStreamIdle
		}
		b.outcome.finish(outcomeErr)
	}
	return n, err
}

func (b *selectedBody) Close() error {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closed = true
		// A consumer can stop at a complete protocol terminal before HTTP EOF.
		// Incomplete, voluntarily closed bodies say nothing about key health.
		if b.observation.Complete() {
			b.outcome.sel.Settle(b.observation.Finish())
		} else if cause := context.Cause(b.outcome.ctx); cause != nil {
			b.outcome.finish(cause)
		} else {
			b.outcome.sel.Release()
		}
		b.mu.Unlock()
		b.closeErr = b.ReadCloser.Close()
	})
	return b.closeErr
}
