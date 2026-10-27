package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

// A stream that dies after the first bytes but before reporting usage is the
// failure a reader most needs to find: the caller received a truncated answer
// and nothing was billed. It used to be skipped entirely, so the dashboard
// reported a 100% streaming success rate while the journal carried the failure.
func TestTruncatedStreamWithoutUsageIsRecorded(t *testing.T) {
	// message_start is enough to put bytes on the wire; it carries no usage.
	const bodyText = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic\"}}\n\n"
	body := &noUsageFailureBody{
		Reader: &dataThenErrorReader{data: []byte(bodyText), delay: 15 * time.Millisecond},
	}
	db, h := newFailureTestHandler(t, body)

	w := httptest.NewRecorder()
	h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
		&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 64},
		[]config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
		nil, router.ScenarioDefault, "no-usage-failure")

	rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("rows = %d (total %d), want the truncated stream recorded once", len(rows), total)
	}
	rec := rows[0]
	if rec.Success {
		t.Fatal("a truncated stream was recorded as a success")
	}
	// The point of recording it is that the dashboard can see the failure.
	// DetailsKnown false is what keeps the empty token figures out of every
	// average: this row is an outcome, not a measurement.
	if rec.DetailsKnown {
		t.Error("a row with no observed usage claims its details were known; " +
			"it would be averaged into success rate and throughput")
	}
	if rec.CostKnown {
		t.Error("an unobserved row carries a cost - " +
			"a stored zero is indistinguishable from a free request")
	}
}

// The other half of that rule: a client that hangs up before any payload
// arrived did not observe a failure. Recording it would turn every interrupted
// prompt into a failed request.
//
// The cancellation has to land *while* the stream is being read. Cancelling
// before the request starts never reaches the disconnect branch at all - the
// handler sees a dead context and returns early - so that shape would pass
// whether or not the rule exists. The probe that established this: marking the
// recording line showed it was never printed for an already-cancelled context.
func TestClientCancelBeforePayloadIsNotRecorded(t *testing.T) {
	reader := &cancelDuringReadReader{after: 30 * time.Millisecond}
	db, h := newFailureTestHandler(t, &noUsageFailureBody{Reader: reader})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader.cancel = cancel

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleStreaming(w, req,
		&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 64},
		[]config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
		nil, router.ScenarioDefault, "client-cancel")

	_, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("recorded %d rows for a client cancellation that never reached the wire, want 0", total)
	}
}

// cancelDuringReadReader cancels the request's context while the handler is
// blocked in Read, which is what a client hanging up mid-request looks like
// from inside the stream loop.
type cancelDuringReadReader struct {
	after  time.Duration
	cancel context.CancelFunc
	done   bool
}

func (r *cancelDuringReadReader) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		time.Sleep(r.after)
		if r.cancel != nil {
			r.cancel()
		}
	}
	return 0, context.Canceled
}

// A measured success must keep details_known = 1 even though RequestRecord's
// zero value for that field is false. Keying the column on DetailsKnown would
// mark every successful request unobserved.
func TestSuccessfulStreamStaysObserved(t *testing.T) {
	db, h := newFailureTestHandler(t, &noUsageFailureBody{Reader: &cleanStreamReader{
		data: []byte(
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic\",\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"),
	}})

	w := httptest.NewRecorder()
	h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
		&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 64},
		[]config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
		nil, router.ScenarioDefault, "clean-success")

	rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("rows = %d (total %d), want 1", len(rows), total)
	}
	if !rows[0].Success {
		t.Fatal("a clean stream was not recorded as a success")
	}
	if !rows[0].DetailsKnown {
		t.Error("a successful request was stored as unobserved; every average would skip it")
	}
}

// cleanStreamReader delivers its data and then ends normally.
type cleanStreamReader struct{ data []byte }

func (r *cleanStreamReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func newFailureTestHandler(t *testing.T, body io.ReadCloser) (*storage.Database, *MessagesHandler) {
	t.Helper()
	cfg := config.NewAtomicConfig(&config.Config{}, "")
	reg := core.NewProviderRegistry()
	if err := reg.Register(&noUsageProvider{body: body}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "f.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, &MessagesHandler{
		fallbackHandler:  router.NewFallbackHandler(nil, 3, time.Minute),
		client:           client.NewOpenCodeClient(cfg),
		providerRegistry: reg,
		streamProxy:      NewStreamProxy(),
		logger:           slog.Default(),
		metrics:          metrics.New(),
		storage:          NewStorageAdapter(db),
	}
}

type noUsageProvider struct{ body io.ReadCloser }

func (p *noUsageProvider) Name() string { return "opencode-zen" }
func (p *noUsageProvider) WireFormat(string) core.WireFormat {
	return core.WireFormatAnthropic
}
func (p *noUsageProvider) Execute(context.Context, *types.MessageRequest, config.ModelConfig) (*core.ExecuteResult, error) {
	return nil, errors.New("not used")
}
func (p *noUsageProvider) Stream(context.Context, *types.MessageRequest, config.ModelConfig) (io.ReadCloser, error) {
	return p.body, nil
}

type noUsageFailureBody struct{ io.Reader }

func (b *noUsageFailureBody) Close() error { return nil }

// blockingThenErrorReader stalls until the caller's context cancels it.
type blockingThenErrorReader struct{}

func (r *blockingThenErrorReader) Read(p []byte) (int, error) {
	time.Sleep(200 * time.Millisecond)
	return 0, context.Canceled
}
