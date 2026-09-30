package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

func TestNonStreamingResponsesFailureRetainsEachAttemptUsage(t *testing.T) {
	names := []string{"opencode-go", "opencode-zen", "aws-bedrock"}
	for index, name := range names {
		fallbackProvider := names[(index+1)%len(names)]
		for _, status := range []string{"failed", "cancelled"} {
			for _, usageMode := range []string{"missing", "null", "zero", "present"} {
				for _, fallbackSuccess := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/usage=%s/fallback=%t", name, status, usageMode, fallbackSuccess), func(t *testing.T) {
						var calls atomic.Int32
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls.Add(1)
							var request types.ResponsesRequest
							if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
								t.Error(err)
								return
							}
							if request.Model == "fallback" && fallbackSuccess {
								_, _ = io.WriteString(w, `{"id":"success","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":5,"output_tokens":2}}`)
								return
							}
							usage := ""
							if usageMode == "present" {
								usage = `,"usage":{"input_tokens":101,"output_tokens":7,"input_tokens_details":{"cached_tokens":90}}`
							}
							if usageMode == "zero" {
								usage = `,"usage":{"input_tokens":0,"output_tokens":0}`
							}
							if usageMode == "null" {
								usage = `,"usage":null`
							}
							_, _ = fmt.Fprintf(w, `{"id":"synthetic","status":%q,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"partial"}]}]%s}`, status, usage)
						}))
						defer upstream.Close()
						cfg := config.NewAtomicConfig(&config.Config{
							OpenCodeGo:  config.OpenCodeGoConfig{APIKey: "synthetic", ResponsesBaseURL: upstream.URL},
							OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic", ResponsesBaseURL: upstream.URL},
							AWSBedrock:  config.AWSBedrockConfig{APIKey: "synthetic", BaseURL: upstream.URL},
						}, "")
						db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(t.TempDir(), "synthetic.db"), RetentionDays: -1})
						if err != nil {
							t.Fatal(err)
						}
						defer func() { _ = db.Close() }()
						logger := slog.New(slog.NewTextHandler(io.Discard, nil))
						h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: newTestProviderRegistry(t, cfg), fallbackHandler: router.NewFallbackHandler(logger, 3, time.Minute), logger: logger, metrics: metrics.New(), storage: NewStorageAdapter(db)}
						skipped := config.ModelConfig{Provider: name, ModelID: "open-circuit", WireFormat: "responses"}
						for range 3 {
							h.fallbackHandler.AllowAttempt(skipped)(context.Background(), &client.APIError{StatusCode: http.StatusServiceUnavailable})
						}
						w := httptest.NewRecorder()
						h.handleNonStreaming(w, httptest.NewRequest(http.MethodPost, "/v1/messages", nil), &types.MessageRequest{Model: "requested", MaxTokens: 16}, []config.ModelConfig{
							skipped,
							{Provider: name, ModelID: "synthetic", WireFormat: "responses"},
							{Provider: fallbackProvider, ModelID: "fallback", WireFormat: "responses"},
						}, nil, router.ScenarioDefault, "synthetic-correlation")
						if fallbackSuccess {
							if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "answer") {
								t.Errorf("fallback success lost: status=%d body=%s", w.Code, w.Body.String())
							}
						} else if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), `"type":"error"`) || strings.Contains(w.Body.String(), "partial") {
							t.Errorf("failed result became success: status=%d body=%s", w.Code, w.Body.String())
						}
						if calls.Load() != 2 {
							t.Errorf("fallback policy changed: calls=%d", calls.Load())
						}
						snapshot := h.metrics.GetSnapshot()
						if fallbackSuccess && (snapshot.RequestsSuccess != 1 || snapshot.RequestsFailed != 0) || !fallbackSuccess && (snapshot.RequestsFailed != 1 || snapshot.RequestsSuccess != 0) {
							t.Errorf("request metrics: %+v", snapshot)
						}
						rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
						if err != nil {
							t.Fatal(err)
						}
						wantRows := int64(2)
						if usageMode == "missing" || usageMode == "null" {
							wantRows = 0
							if fallbackSuccess {
								wantRows = 1
							}
						}
						if total != wantRows {
							t.Fatalf("attempt usage rows=%d want=%d: %+v", total, wantRows, rows)
						}
						ids := make(map[string]bool)
						for _, row := range rows {
							if ids[row.ID] || row.ID == "synthetic-correlation" {
								t.Errorf("execution ID reused: %s", row.ID)
							}
							ids[row.ID] = true
							wantAttempt := 2
							wantProvider := name
							if row.Model == "fallback" {
								wantAttempt = 3
								wantProvider = fallbackProvider
							}
							if row.Streaming || row.Provider != wantProvider || row.RequestedModel != "requested" {
								t.Errorf("identity mismatch: %+v", row)
							}
							if row.Attempt != wantAttempt {
								t.Errorf("attempt ordinal mismatch: %+v", row)
							}
							if row.Success {
								if !fallbackSuccess || row.Model != "fallback" || row.InputTokens != 5 || row.OutputTokens != 2 {
									t.Errorf("success accounting mismatch: %+v", row)
								}
								continue
							}
							if !strings.Contains(row.ErrorMsg, status) {
								t.Errorf("failure status lost: %+v", row)
							}
							wantIn, wantRead, wantOut := 0, 0, 0
							if usageMode == "present" {
								wantIn, wantRead, wantOut = 11, 90, 7
							}
							if row.InputTokens != wantIn || row.CacheReadTokens != wantRead || row.OutputTokens != wantOut {
								t.Errorf("failure usage mismatch: %+v", row)
							}
						}
					})
				}
			}
		}
	}
}
