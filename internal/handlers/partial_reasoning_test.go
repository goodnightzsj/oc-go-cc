package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

type chatUsageProvider struct{ noUsageProvider }

func (*chatUsageProvider) WireFormat(string) core.WireFormat { return core.WireFormatOpenAIChat }

type responsesUsageProvider struct{ noUsageProvider }

func (*responsesUsageProvider) WireFormat(string) core.WireFormat {
	return core.WireFormatOpenAIResponses
}

func TestResponsesInterruptedStreamRecordsReasoning(t *testing.T) {
	const wire = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\",\"usage\":{\"input_tokens\":20,\"output_tokens\":36,\"output_tokens_details\":{\"reasoning_tokens\":26}}}\n\n"
	body := io.NopCloser(&dataThenErrorReader{data: []byte(wire)})
	db, h := newFailureTestHandler(t, body)
	h.providerRegistry = core.NewProviderRegistry()
	if err := h.providerRegistry.Register(&responsesUsageProvider{noUsageProvider{body: body}}); err != nil {
		t.Fatal(err)
	}
	h.handleStreaming(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
		&types.MessageRequest{Stream: boolPtr(true)}, []config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
		nil, router.ScenarioDefault, "interrupted-responses")
	rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("rows=%+v total=%d err=%v", rows, total, err)
	}
	if r := rows[0]; r.Success || r.InputTokens != 20 || r.OutputTokens != 36 || r.ReasoningTokens != 26 {
		t.Fatalf("interrupted Responses accounting = %+v", r)
	}
}

func TestStreamRecordsReasoningBeforeInterruption(t *testing.T) {
	const wire = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":36,\"completion_tokens_details\":{\"reasoning_tokens\":26}}}\n\n"
	for _, interrupted := range []bool{false, true} {
		name := "completed"
		if interrupted {
			name = "interrupted"
		}
		t.Run(name, func(t *testing.T) {
			var reader io.Reader = &cleanStreamReader{data: []byte(wire)}
			if interrupted {
				reader = &dataThenErrorReader{data: []byte(wire)}
			}
			body := io.NopCloser(reader)
			db, h := newFailureTestHandler(t, body)
			h.providerRegistry = core.NewProviderRegistry()
			if err := h.providerRegistry.Register(&chatUsageProvider{noUsageProvider{body: body}}); err != nil {
				t.Fatal(err)
			}
			h.handleStreaming(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", nil),
				&types.MessageRequest{Stream: boolPtr(true)}, []config.ModelConfig{{Provider: "opencode-zen", ModelID: "synthetic"}},
				nil, router.ScenarioDefault, name)
			rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{})
			if err != nil || total != 1 || len(rows) != 1 {
				t.Fatalf("rows=%+v total=%d err=%v", rows, total, err)
			}
			r := rows[0]
			if r.Success == interrupted || r.InputTokens != 20 || r.OutputTokens != 36 || r.ReasoningTokens != 26 {
				t.Fatalf("stream accounting = %+v", r)
			}
		})
	}
}
