package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

func TestNativeAnthropicContentAfterTerminal(t *testing.T) {
	for _, mode := range []string{"normal", "content", "start", "usage", "ping", "extension"} {
		t.Run(mode, func(t *testing.T) {
			wire := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic\",\"usage\":{\"input_tokens\":1}}}\n\n" +
				"data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n" +
				"data: {\"type\":\"message_stop\"}\n\n"
			var tail string
			switch mode {
			case "content":
				tail = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"late"}}`
			case "start":
				tail = `{"type":"message_start","message":{"model":"late"}}`
			case "usage":
				tail = `{"type":"message_delta","usage":{"output_tokens":3,"service_tier":"standard"}}`
			case "ping":
				tail = `{"type":"ping"}`
			case "extension":
				tail = `{"type":"future_extension","opaque":{"signed":true}}`
			}
			if tail != "" {
				wire += "data: " + tail + "\n\n"
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, wire)
			}))
			defer upstream.Close()
			cfg := config.NewAtomicConfig(&config.Config{CommandCode: config.CommandCodeConfig{APIKey: "synthetic-only", AnthropicBaseURL: upstream.URL}}, "")
			records := &timingRecordStore{}
			h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: newTestProviderRegistry(t, cfg), fallbackHandler: router.NewFallbackHandler(nil, 3, time.Minute), metrics: metrics.New(), streamProxy: NewStreamProxy(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), storage: records}
			out := httptest.NewRecorder()
			h.handleStreaming(out, httptest.NewRequest(http.MethodPost, "/v1/messages", nil), &types.MessageRequest{Model: "m", MaxTokens: 16, Stream: boolPtr(true)}, []config.ModelConfig{{Provider: "commandcode", ModelID: "synthetic", WireFormat: "anthropic"}}, nil, router.ScenarioDefault, "synthetic")
			failed := mode == "content" || mode == "start"
			if len(records.rows) != 1 || records.rows[0].Success == failed {
				t.Fatalf("terminal accounting changed: records=%+v output=%s", records.rows, out.Body.String())
			}
			if failed && (!strings.Contains(out.Body.String(), "event: error") || strings.Contains(out.Body.String(), tail)) {
				t.Fatalf("late content was forwarded or error lost: %s", out.Body.String())
			}
		})
	}
}

func TestAnthropicTerminalObservedFragments(t *testing.T) {
	for _, size := range []int{1, 7, 4096} {
		out := httptest.NewRecorder()
		w := &responseWriter{ResponseWriter: out}
		wire := []byte("data: {\"type\":\"message_stop\"}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"late\"}}\n\n")
		var err error
		for len(wire) > 0 && err == nil {
			n := min(size, len(wire))
			_, err = w.Write(wire[:n])
			wire = wire[n:]
		}
		if err == nil || w.finish() == nil {
			t.Fatalf("observed late content accepted with chunk size %d", size)
		}
	}
}
