package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
)

func TestNativeResponsesReasoningReachesBothProtocolsAndHistory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var req struct{ Stream bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		response := `{"id":"resp_synthetic","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":101,"output_tokens":36,"total_tokens":137,"input_tokens_details":{"cached_tokens":90},"output_tokens_details":{"reasoning_tokens":26}}}`
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", response)
			return
		}
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(upstream.Close)
	respect := false
	srv, err := NewServer(config.NewAtomicConfig(&config.Config{
		ActiveSite:            "opencode-go",
		OpenCodeGo:            config.OpenCodeGoConfig{ResponsesBaseURL: upstream.URL + "/v1/responses", APIKey: "synthetic-go-key"},
		Models:                map[string]config.ModelConfig{"default": {Provider: "opencode-go", ModelID: "synthetic-responses", WireFormat: "responses", MaxTokens: 100}},
		RespectRequestedModel: &respect,
		Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
	}, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", path, stream), func(t *testing.T) {
				payload := fmt.Sprintf(`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
				if path == "/v1/responses" {
					payload = fmt.Sprintf(`{"model":"m","input":"hi","stream":%t}`, stream)
				}
				w := httptest.NewRecorder()
				srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
				if w.Code != 200 {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
				raw := w.Body.Bytes()
				if stream {
					raw = nil
					for _, line := range strings.Split(w.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: ") {
							continue
						}
						data := []byte(strings.TrimPrefix(line, "data: "))
						var event struct {
							Type     string
							Response json.RawMessage
						}
						if err := json.Unmarshal(data, &event); err != nil {
							t.Fatal(err)
						}
						if event.Type == "message_delta" {
							raw = data
						}
						if event.Type == "response.completed" {
							raw = event.Response
						}
					}
				}
				var result struct {
					Usage struct {
						Output    int            `json:"output_tokens"`
						Reasoning int            `json:"reasoning_tokens"`
						Details   map[string]int `json:"output_tokens_details"`
					}
				}
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				if result.Usage.Output != 36 {
					t.Fatalf("output=%d", result.Usage.Output)
				}
				reasoning := result.Usage.Reasoning
				if path == "/v1/responses" {
					reasoning = result.Usage.Details["reasoning_tokens"]
				}
				if reasoning != 26 {
					t.Errorf("reasoning=%d, want 26; terminal=%s", reasoning, raw)
				}
			})
		}
	}
	rows, total, err := storage.NewRequests(srv.Storage()).Query(storage.RequestQuery{})
	if err != nil || total != 4 {
		t.Fatalf("history count=%d err=%v", total, err)
	}
	for _, row := range rows {
		if row.InputTokens != 11 || row.CacheReadTokens != 90 || row.OutputTokens != 36 {
			t.Errorf("wrong totals: input=%d cache=%d output=%d", row.InputTokens, row.CacheReadTokens, row.OutputTokens)
		}
		if row.ReasoningTokens != 26 {
			t.Errorf("history stream=%t reasoning=%d, want 26", row.Streaming, row.ReasoningTokens)
		}
		if row.KeyHint != "****-key" {
			t.Errorf("history stream=%t key_hint=%q, want masked credential label", row.Streaming, row.KeyHint)
		}
		if row.Streaming {
			if row.FirstEventMs == nil || row.FirstContentMs == nil || row.MessageEndMs == nil {
				t.Error("real HTTP streaming path lost timing")
			}
		} else if row.FirstEventMs != nil || row.FirstContentMs != nil || row.MessageEndMs != nil {
			t.Error("non-streaming HTTP path fabricated stream timing")
		}
	}
}
