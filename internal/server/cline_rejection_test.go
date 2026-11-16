package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestClineTruncatedRejectionReachesBothProtocols(t *testing.T) {
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			for _, status := range []int{401, 403, 429} {
				t.Run(fmt.Sprintf("%s/stream=%t/http=%d", path, stream, status), func(t *testing.T) {
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("Content-Length", "1000")
						w.Header().Set("Retry-After", "19")
						w.WriteHeader(status)
						_, _ = io.WriteString(w, "synthetic refusal")
					}))
					t.Cleanup(upstream.Close)
					respect := false
					srv, err := NewServer(config.NewAtomicConfig(&config.Config{
						ActiveSite:            "cline-pass",
						ClinePass:             config.ClinePassConfig{APIKey: "synthetic", BaseURL: upstream.URL},
						Models:                map[string]config.ModelConfig{"default": {Provider: "cline-pass", ModelID: "synthetic", MaxTokens: 100}},
						RespectRequestedModel: &respect,
						Storage:               &config.StorageConfig{DatabasePath: filepath.Join(t.TempDir(), "history.db"), RetentionDays: -1},
					}, ""), nil)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = srv.Close() })
					payload := fmt.Sprintf(`{"model":"alias","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"stream":%t}`, stream)
					if path == "/v1/responses" {
						payload = fmt.Sprintf(`{"model":"alias","input":"hi","stream":%t}`, stream)
					}
					out := httptest.NewRecorder()
					srv.httpSrv.Handler.ServeHTTP(out, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
					body := out.Body.String()
					if !strings.Contains(body, fmt.Sprintf("HTTP %d", status)) {
						t.Fatalf("lost source refusal: status=%d body=%s", out.Code, body)
					}
					if !stream {
						if out.Code != status {
							t.Fatalf("source HTTP%d replaced with %d", status, out.Code)
						}
					} else if !strings.Contains(body, "event: error") && !strings.Contains(body, "event: response.failed") {
						t.Fatalf("missing failed stream: %s", body)
					}
					if strings.Contains(body, "event: message_stop") || strings.Contains(body, "event: response.completed") {
						t.Fatal("refusal reported as a successful terminal")
					}
				})
			}
		}
	}
}
