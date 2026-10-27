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

func TestGeminiUsageReachesBothProtocolsAndHistory(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-3.8-flash" {
			t.Errorf("unexpected Gemini path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var req struct{ Stream bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		const response = `{"modelVersion":"gemini-synthetic-reported","candidates":[{"content":{"role":"model","parts":[{"text":"plan","thought":true},{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":101,"cachedContentTokenCount":90,"candidatesTokenCount":10,"thoughtsTokenCount":26,"totalTokenCount":137}}`
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", response)
		} else {
			_, _ = io.WriteString(w, response)
		}
	}))
	t.Cleanup(upstream.Close)
	respect := false
	srv, err := NewServer(config.NewAtomicConfig(&config.Config{
		ActiveSite:            "opencode-zen",
		OpenCodeZen:           config.OpenCodeZenConfig{GeminiBaseURL: upstream.URL + "/models", APIKey: "synthetic-zen-key"},
		Models:                map[string]config.ModelConfig{"default": {Provider: "opencode-zen", ModelID: "gemini-3.8-flash", MaxTokens: 100}},
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
						Input         int            `json:"input_tokens"`
						Output        int            `json:"output_tokens"`
						Cache         int            `json:"cache_read_input_tokens"`
						Reasoning     int            `json:"reasoning_tokens"`
						InputDetails  map[string]int `json:"input_tokens_details"`
						OutputDetails map[string]int `json:"output_tokens_details"`
					}
				}
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				u := result.Usage
				if path == "/v1/responses" {
					u.Cache = u.InputDetails["cached_tokens"]
					u.Input -= u.Cache
					u.Reasoning = u.OutputDetails["reasoning_tokens"]
				}
				if u.Input != 11 || u.Output != 36 || u.Cache != 90 || u.Reasoning != 26 {
					t.Fatalf("usage=%+v", u)
				}
			})
		}
	}
	rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
	if err != nil || total != 4 {
		t.Fatalf("history total=%d err=%v", total, err)
	}
	for _, row := range rows {
		if row.Model != "gemini-3.8-flash" || row.UpstreamModel != "gemini-synthetic-reported" {
			t.Fatalf("reported model lost or changed billing identity: %+v", row)
		}
		if row.InputTokens != 11 || row.CacheReadTokens != 90 || row.OutputTokens != 36 || row.ReasoningTokens != 26 {
			t.Errorf("history lost Gemini counts: %+v", row)
		}
	}
}
