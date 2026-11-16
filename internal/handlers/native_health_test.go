package handlers

import (
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

func TestInvalidNativeUsageAffectsKeyHealth(t *testing.T) {
	var mu sync.Mutex
	var used []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		used = append(used, r.Header.Get("Authorization"))
		mu.Unlock()
		_, _ = io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic\",\"usage\":{\"input_tokens\":\"oops\"}}}\n\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer upstream.Close()
	cfg := config.NewAtomicConfig(&config.Config{CommandCode: config.CommandCodeConfig{APIKeys: []string{"synthetic-a", "synthetic-b"}, AnthropicBaseURL: upstream.URL}}, "")
	reg := core.NewProviderRegistry()
	_ = reg.Register(provider.NewCommandCodeProvider(cfg, nil))
	h := &MessagesHandler{client: client.NewOpenCodeClient(cfg), providerRegistry: reg, fallbackHandler: router.NewFallbackHandler(nil, 99, time.Minute), streamProxy: NewStreamProxy(), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), metrics: metrics.New()}
	for range 4 {
		w := httptest.NewRecorder()
		h.handleStreaming(w, httptest.NewRequest("POST", "/v1/messages", nil), &types.MessageRequest{Stream: boolPtr(true), MaxTokens: 16}, []config.ModelConfig{{Provider: "commandcode", ModelID: "synthetic", WireFormat: "anthropic"}}, nil, router.ScenarioDefault, "synthetic")
		if !strings.Contains(w.Body.String(), "event: error") {
			t.Fatalf("malformed stream unexpectedly succeeded: %s", w.Body.String())
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(used) != 4 || used[3] != "Bearer synthetic-b" {
		t.Fatalf("malformed native source is credited healthy: used=%v", used)
	}
}
