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
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/storage"
)

func TestWorkBuddyReasoningReachesBothProtocolsAndHistory(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		want         int
		trailer      string
	}{
		{"details", `,"completion_tokens_details":{"reasoning_tokens":26}`, 26, ""},
		{"thinking", `,"completion_thinking_tokens":26`, 26, ""},
		{"both", `,"completion_tokens_details":{"reasoning_tokens":26},"completion_thinking_tokens":5`, 26, ""},
		{"unobserved", ``, 0, ""},
		{"later totals omit split", `,"completion_tokens_details":{"reasoning_tokens":26}`, 26, `"prompt_tokens":101,"completion_tokens":36,"total_tokens":137,"prompt_tokens_details":{"cached_tokens":90}`},
		{"later zero split", `,"completion_thinking_tokens":26`, 26, `"prompt_tokens":101,"completion_tokens":36,"total_tokens":137,"prompt_tokens_details":{"cached_tokens":90},"completion_tokens_details":{"reasoning_tokens":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-workbuddy" {
					t.Error("WorkBuddy request lost its provider credential")
				}
				if r.URL.Path == "/v1/models" {
					_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"global:synthetic","object":"model","owned_by":"workbuddy"}]}`)
					return
				}
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected WorkBuddy request: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				var request struct{ Stream bool }
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				usage := `"usage":{"prompt_tokens":101,"completion_tokens":36,"total_tokens":137,"prompt_tokens_details":{"cached_tokens":90}` + tc.fields + `}`
				if request.Stream {
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],"+usage+"}\n\n")
					if tc.trailer != "" {
						_, _ = fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{%s}}\n\n", tc.trailer)
					}
					_, _ = io.WriteString(w, "data: [DONE]\n\n")
					return
				}
				_, _ = io.WriteString(w, `{"id":"synthetic","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],`+usage+`}`)
			}))
			t.Cleanup(upstream.Close)
			dir := t.TempDir()
			captureStore, err := debug.NewStorage(config.DebugCapture{Enabled: true, Directory: filepath.Join(dir, "capture"), MaxFiles: 2, MaxFileSize: 1 << 20})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = captureStore.Close() })
			capture := debug.NewCaptureLogger(captureStore, true)
			t.Cleanup(func() { _ = capture.Close() })
			respect := false
			srv, err := NewServer(config.NewAtomicConfig(&config.Config{
				ActiveSite: "workbuddy", WorkBuddy: config.WorkBuddyConfig{BaseURL: upstream.URL, APIKey: "synthetic-workbuddy"},
				Models:                map[string]config.ModelConfig{"default": {Provider: "workbuddy", ModelID: "global:synthetic", MaxTokens: 100}},
				RespectRequestedModel: &respect,
				Storage:               &config.StorageConfig{DatabasePath: filepath.Join(dir, "history.db"), RetentionDays: -1},
			}, ""), capture)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = srv.Close() })
			for _, path := range []string{"/v1/messages", "/v1/responses"} {
				for _, stream := range []bool{false, true} {
					payload := fmt.Sprintf(`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
					if path == "/v1/responses" {
						payload = fmt.Sprintf(`{"model":"m","input":"hi","stream":%t}`, stream)
					}
					w := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
					if w.Code != http.StatusOK {
						t.Fatalf("%s stream=%t: HTTP %d %s", path, stream, w.Code, w.Body.String())
					}
					raw := w.Body.Bytes()
					if stream {
						raw = nil
						for _, line := range strings.Split(w.Body.String(), "\n") {
							if !strings.HasPrefix(line, "data: ") {
								continue
							}
							var event struct {
								Type     string
								Response json.RawMessage
							}
							data := []byte(strings.TrimPrefix(line, "data: "))
							if err := json.Unmarshal(data, &event); err != nil {
								t.Fatal(err)
							}
							if event.Type == "message_delta" {
								raw = data
							} else if event.Type == "response.completed" {
								raw = event.Response
							}
						}
					}
					var response struct {
						Usage struct {
							Input     int            `json:"input_tokens"`
							Output    int            `json:"output_tokens"`
							Total     int            `json:"total_tokens"`
							Cache     int            `json:"cache_read_input_tokens"`
							Reasoning *int           `json:"reasoning_tokens"`
							Details   map[string]int `json:"output_tokens_details"`
						} `json:"usage"`
					}
					if err := json.Unmarshal(raw, &response); err != nil {
						t.Fatalf("%s stream=%t: terminal response missing: %v", path, stream, err)
					}
					got := response.Usage
					if got.Output != 36 {
						t.Fatalf("output includes reasoning twice: %+v", got)
					}
					if path == "/v1/messages" {
						if got.Input != 11 || got.Cache != 90 || (tc.want == 0 && got.Reasoning != nil) || (tc.want > 0 && (got.Reasoning == nil || *got.Reasoning != tc.want)) {
							t.Fatalf("Messages stream=%t lost usage: %+v", stream, got)
						}
					} else if got.Input != 101 || got.Total != 137 || got.Details["reasoning_tokens"] != tc.want || (tc.want == 0 && got.Details != nil) {
						t.Fatalf("Responses stream=%t lost usage: %+v", stream, got)
					}
				}
			}
			rows, total, err := storage.NewRequests(srv.Storage()).Query(storage.RequestQuery{})
			if err != nil || total != 4 || len(rows) != 4 {
				t.Fatalf("history rows=%d total=%d err=%v", len(rows), total, err)
			}
			for _, row := range rows {
				if !row.Success || row.Provider != "workbuddy" || row.InputTokens != 11 || row.CacheReadTokens != 90 || row.OutputTokens != 36 || row.ReasoningTokens != tc.want {
					t.Fatalf("WorkBuddy history lost usage: %+v", row)
				}
			}
		})
	}
}
