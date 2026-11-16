package handlers

import (
	"encoding/json"
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

func TestNativeAnthropicFailureAccounting(t *testing.T) {
	for _, name := range []string{"opencode-go", "opencode-zen", "aws-bedrock", "commandcode"} {
		for _, payload := range []string{
			`{"type":"error","error":{"type":"overloaded_error","message":"synthetic"},"usage":{"input_tokens":2}}`,
			`{"type":`,
			`{"type":"message","content":[],"stop_reason":null}`,
		} {
			t.Run(name+"/"+payload, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, payload) }))
				defer upstream.Close()
				keys := []string{"synthetic-a", "synthetic-b"}
				cfg := config.NewAtomicConfig(&config.Config{
					APIKeys:     keys,
					OpenCodeGo:  config.OpenCodeGoConfig{AnthropicBaseURL: upstream.URL},
					OpenCodeZen: config.OpenCodeZenConfig{AnthropicBaseURL: upstream.URL},
					AWSBedrock:  config.AWSBedrockConfig{AnthropicBaseURL: upstream.URL},
					CommandCode: config.CommandCodeConfig{AnthropicBaseURL: upstream.URL, APIKeys: keys},
				}, "")
				store := &timingRecordStore{}
				h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: newTestProviderRegistry(t, cfg), fallbackHandler: router.NewFallbackHandler(nil, 99, time.Minute), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), metrics: metrics.New(), storage: store}
				w := httptest.NewRecorder()
				h.handleNonStreaming(w, httptest.NewRequest("POST", "/v1/messages", nil), &types.MessageRequest{Model: "synthetic", MaxTokens: 16, Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}}, []config.ModelConfig{{Provider: name, ModelID: "synthetic", WireFormat: "anthropic"}}, nil, router.ScenarioDefault, "synthetic")
				snapshot := h.metrics.GetSnapshot()
				if w.Code == http.StatusOK || snapshot.RequestsSuccess != 0 || snapshot.RequestsFailed != 1 {
					t.Fatalf("invalid native body counted as success: status=%d metrics=%+v", w.Code, snapshot)
				}
				if len(store.rows) != 1 || store.rows[0].Success {
					t.Fatalf("expected one failed attempt: %+v", store.rows)
				}
				if strings.Contains(payload, "input_tokens") {
					if store.rows[0].InputTokens != 2 || store.rows[0].UsageUnobserved {
						t.Fatalf("reported failure usage lost: %+v", store.rows[0])
					}
				} else if !store.rows[0].UsageUnobserved {
					t.Fatalf("unobserved usage fabricated: %+v", store.rows[0])
				}
			})
		}
	}
}
