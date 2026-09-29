package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/storage"
)

func TestOpenRouterRegisteredRoutingAccountingAndCapture(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Stream bool }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		const usage = `"usage":{"prompt_tokens":101,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":90}}`
		if req.Stream {
			_, _ = io.WriteString(w, `data: {"id":"c","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}],`+usage+"}\n\ndata: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(w, `{"id":"c","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],`+usage+`}`)
	}))
	defer upstream.Close()
	dir := t.TempDir()
	captureStore, err := debug.NewStorage(config.DebugCapture{Enabled: true, Directory: dir, MaxFiles: 2, MaxFileSize: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = captureStore.Close() }()
	capture := debug.NewCaptureLogger(captureStore, true)
	defer func() { _ = capture.Close() }()
	respect := false
	srv, err := NewServer(config.NewAtomicConfig(&config.Config{
		OpenRouter:            config.OpenRouterConfig{BaseURL: upstream.URL, APIKey: "synthetic-openrouter"},
		Models:                map[string]config.ModelConfig{"default": {Provider: "openrouter", ModelID: "synthetic", MaxTokens: 100}},
		RespectRequestedModel: &respect,
		Storage:               &config.StorageConfig{DatabasePath: filepath.Join(dir, "test.db"), RetentionDays: -1},
	}, ""), capture)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			payload := fmt.Sprintf(`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
			if path == "/v1/responses" {
				payload = fmt.Sprintf(`{"model":"m","input":"hi","stream":%t}`, stream)
			}
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
			req.Header.Set("X-Request-ID", "synthetic-reused-id")
			w := httptest.NewRecorder()
			srv.httpSrv.Handler.ServeHTTP(w, req)
			if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"type":"error"`) || strings.Contains(w.Body.String(), `"status":"failed"`) || w.Header().Get("X-Request-ID") != "synthetic-reused-id" {
				t.Fatalf("%s stream=%t: %d %s", path, stream, w.Code, w.Body.String())
			}
		}
	}
	rows, total, err := storage.NewRequests(srv.Storage()).Query(storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 4 {
		t.Fatalf("accounting count=%d err=%v", total, err)
	}
	for _, row := range rows {
		if row.Provider != "openrouter" || !row.Success || row.InputTokens != 11 || row.CacheReadTokens != 90 || row.OutputTokens != 7 {
			t.Fatalf("accounting changed: %+v", row)
		}
	}
	if err := capture.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "capture-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("capture files=%v err=%v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	phases := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var entry debug.CaptureEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Phase == debug.PhaseUpstreamRequest || entry.Phase == debug.PhaseUpstreamResponse {
			if entry.Provider != "openrouter" || entry.RequestID != "synthetic-reused-id" {
				t.Fatalf("wrong capture correlation: %+v", entry)
			}
			phases[entry.Phase]++
		}
	}
	if phases[debug.PhaseUpstreamRequest] != 4 || phases[debug.PhaseUpstreamResponse] != 4 {
		t.Fatalf("capture incomplete: %v", phases)
	}
}
