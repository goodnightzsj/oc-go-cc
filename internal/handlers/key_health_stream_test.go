package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/pkg/types"
)

type disconnectOnContent struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	fail   bool
}

func (w *disconnectOnContent) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("partial")) {
		if w.fail {
			return 0, io.ErrClosedPipe
		}
		w.cancel()
	}
	return w.ResponseRecorder.Write(p)
}

func TestKeyHealthTracksSourceNotDownstream(t *testing.T) {
	for _, cause := range []string{"idle", "eof", "client_cancel", "write_error"} {
		t.Run(cause, func(t *testing.T) {
			var mu sync.Mutex
			var used []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				used = append(used, r.Header.Get("Authorization"))
				mu.Unlock()
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
				w.(http.Flusher).Flush()
				if cause != "eof" {
					<-r.Context().Done()
				}
			}))
			defer upstream.Close()
			cfg := config.NewAtomicConfig(&config.Config{APIKeys: []string{"synthetic-a", "synthetic-b"},
				OpenRouter: config.OpenRouterConfig{BaseURL: upstream.URL, StreamTimeoutMs: 20, StreamingTimeoutMs: 1000}}, "")
			reg := core.NewProviderRegistry()
			if err := reg.Register(provider.NewOpenRouterProvider(cfg, nil)); err != nil {
				t.Fatal(err)
			}
			h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: reg,
				fallbackHandler: router.NewFallbackHandler(nil, 99, time.Minute), streamProxy: NewStreamProxy(),
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)), metrics: metrics.New()}
			for i := 0; i < 4; i++ {
				ctx, cancel := context.WithCancel(context.Background())
				var w http.ResponseWriter = httptest.NewRecorder()
				if cause == "client_cancel" || cause == "write_error" {
					w = &disconnectOnContent{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, fail: cause == "write_error"}
				}
				h.handleStreaming(w, httptest.NewRequest("POST", "/v1/messages", nil).WithContext(ctx),
					&types.MessageRequest{Stream: boolPtr(true), MaxTokens: 16, Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}},
					[]config.ModelConfig{{Provider: "openrouter", ModelID: "synthetic"}}, nil, router.ScenarioDefault, "synthetic")
				cancel()
			}
			mu.Lock()
			got := strings.Join(used, ",")
			mu.Unlock()
			want := "Bearer synthetic-a,Bearer synthetic-a,Bearer synthetic-a,Bearer synthetic-a"
			if cause == "idle" || cause == "eof" {
				want = strings.TrimSuffix(want, "synthetic-a") + "synthetic-b"
			}
			if got != want {
				t.Fatalf("%s: selected %s want %s", cause, got, want)
			}
		})
	}
}
