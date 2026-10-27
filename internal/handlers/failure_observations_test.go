package handlers

import (
	"context"
	"encoding/json"
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

// observedFailureBody emits a chunk carrying usage, then fails - and it
// publishes the same observations a real gateway body does.
type observedFailureBody struct {
	io.Reader
	model       string
	fb          time.Time
	stampOnRead bool
}

func (b *observedFailureBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if b.stampOnRead && n > 0 {
		b.MarkFirstByte()
	}
	return n, err
}

func (b *observedFailureBody) Close() error            { return nil }
func (b *observedFailureBody) UpstreamModel() string   { return b.model }
func (b *observedFailureBody) ServedModel() string     { return b.model }
func (b *observedFailureBody) UpstreamChannel() string { return "deepseek" }
func (b *observedFailureBody) MarkFirstByte()          { b.fb = time.Now() }
func (b *observedFailureBody) FirstByteAt() time.Time  { return b.fb }

type failingObservedProvider struct{ body *observedFailureBody }

func TestRecordedUpstreamModelPrefersGatewayDeclaration(t *testing.T) {
	rw := &responseWriter{}
	rw.SetSourceModel("response-label")
	if got := rw.recordedUpstreamModel(&observedFailureBody{model: "gateway-canonical"}, "requested"); got != "gateway-canonical" {
		t.Fatalf("gateway priority changed: %s", got)
	}
	if got := rw.recordedUpstreamModel(nil, "requested"); got != "response-label" {
		t.Fatalf("wire model lost: %s", got)
	}
	if got := rw.recordedUpstreamModel(nil, "response-label"); got != "" {
		t.Fatalf("identical model duplicated: %s", got)
	}
}

func (p *failingObservedProvider) Name() string { return "opencode-zen" }
func (p *failingObservedProvider) WireFormat(string) core.WireFormat {
	return core.WireFormatAnthropic
}
func (p *failingObservedProvider) Execute(context.Context, *types.MessageRequest, config.ModelConfig) (*core.ExecuteResult, error) {
	return nil, errors.New("not used")
}
func (p *failingObservedProvider) Stream(context.Context, *types.MessageRequest, config.ModelConfig) (io.ReadCloser, error) {
	return p.body, nil
}

// A stream that dies after reporting usage is recorded - and the record must
// carry the observations the success path carries. They were missing, so a
// failed row could not say which model ran, how long the first byte took, or
// what level the client asked for.
func TestInterruptedStreamRecordsTheSameObservationsAsSuccess(t *testing.T) {
	// message_start reports usage, then the body errors before finishing.
	const bodyText = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic\",\"usage\":{\"input_tokens\":9,\"output_tokens\":0}}}\n\n"
	// The error arrives on a read that also carries the last chunk, which is
	// what a reset mid-frame looks like. A separate failing reader would return
	// n=0 and never exercise the stamp at all.
	// A real first byte is always some time after the request started. Without
	// that gap the sub-millisecond duration truncates to zero and the column
	// stores NULL, which is indistinguishable from "never measured" - so the
	// fixture would not be able to tell the two apart either.
	body := &observedFailureBody{
		Reader: &dataThenErrorReader{data: []byte(bodyText), delay: 15 * time.Millisecond},
		model:  "deepseek/real-model",
	}
	// Stamped from inside the stream, after streamStart: a body that stamps
	// before the handler started timing would be reporting an impossible
	// sequence and firstByteDuration rightly discards it.
	body.stampOnRead = true

	cfg := config.NewAtomicConfig(&config.Config{}, "")
	reg := core.NewProviderRegistry()
	if err := reg.Register(&failingObservedProvider{body: body}); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "f.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	h := &MessagesHandler{
		fallbackHandler:  router.NewFallbackHandler(nil, 3, time.Minute),
		client:           client.NewOpenCodeClient(cfg),
		providerRegistry: reg,
		streamProxy:      NewStreamProxy(),
		logger:           slog.Default(),
		metrics:          metrics.New(),
		storage:          NewStorageAdapter(db),
	}
	w := httptest.NewRecorder()
	h.handleStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
		&types.MessageRequest{
			Stream: boolPtr(true), MaxTokens: 64,
			OutputConfig: json.RawMessage(`{"effort":"xhigh"}`),
		},
		[]config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
		nil, router.ScenarioDefault, "failure-observed")

	rows, total, err := storage.NewRequests(db).Query(t.Context(), storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 {
		t.Fatal("the interrupted stream was not recorded at all")
	}
	rec := rows[0]
	if rec.Success {
		t.Fatal("a failed stream was recorded as a success")
	}
	if rec.UpstreamModel != "deepseek/real-model" {
		t.Errorf("UpstreamModel = %q, want the model the body reported", rec.UpstreamModel)
	}
	if rec.ResponseModel != "synthetic" {
		t.Errorf("ResponseModel = %q, want the source message model, not canonical", rec.ResponseModel)
	}
	if rec.FirstByte <= 0 {
		t.Errorf("FirstByte = %v, want the measured first byte", rec.FirstByte)
	}
	if rec.ReasoningEffort != "xhigh" {
		t.Errorf("ReasoningEffort = %q, want the level the client asked for", rec.ReasoningEffort)
	}
}

// dataThenErrorReader delivers its data, then fails.
type dataThenErrorReader struct {
	data  []byte
	delay time.Duration
	first bool
}

func (r *dataThenErrorReader) Read(p []byte) (int, error) {
	if !r.first {
		r.first = true
		time.Sleep(r.delay)
	}
	if len(r.data) == 0 {
		return 0, errors.New("upstream connection reset")
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}
