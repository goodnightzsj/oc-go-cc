package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/debug"
)

// Real capture must preserve observations and persist the bytes actually read.
func TestWorkBuddyReportsKeyHintAndFirstByteThroughCapture(t *testing.T) {
	const wire = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	for _, mode := range []string{"disabled", "enabled"} {
		t.Run(mode, func(t *testing.T) {
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				select {
				case <-release:
					_, _ = io.WriteString(w, wire)
				case <-r.Context().Done():
				}
			}))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			dir := t.TempDir()
			enabled := mode == "enabled"
			store, err := debug.NewStorage(config.DebugCapture{Enabled: enabled, Directory: dir, MaxFileSize: 1 << 20, MaxFiles: 2, RedactAPIKeys: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			capture := debug.NewCaptureLogger(store, enabled)
			t.Cleanup(func() {
				if err := capture.Close(); err != nil {
					t.Error(err)
				}
			})
			if (capture != nil) != enabled {
				t.Fatal("capture setup does not match requested mode")
			}
			cfg := &config.Config{}
			cfg.WorkBuddy.BaseURL, cfg.WorkBuddy.APIKey = srv.URL, "sk-test-abcd"
			p := NewWorkBuddyProvider(config.NewAtomicConfig(cfg, ""), capture)
			body, err := p.Stream(ctx, workBuddyTestRequest(), config.ModelConfig{Provider: "workbuddy", ModelID: "global:x"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = body.Close() })
			if at := client.FirstByteAtOf(body); !at.IsZero() {
				t.Fatalf("headers stamped a first byte: %v", at)
			}
			releasedAt := time.Now()
			close(release)
			first := make([]byte, 1)
			if _, err := io.ReadFull(body, first); err != nil {
				t.Fatal(err)
			}
			at := client.FirstByteAtOf(body)
			if at.IsZero() || at.Before(releasedAt) {
				t.Fatalf("first byte=%v, body released=%v", at, releasedAt)
			}
			rest, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			if string(append(first, rest...)) != wire {
				t.Fatal("capture changed response bytes")
			}
			if client.FirstByteAtOf(body) != at {
				t.Fatal("subsequent reads changed the first-byte stamp")
			}
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}
			if hint := client.KeyHintOf(body); hint == "" || strings.Contains(hint, cfg.WorkBuddy.APIKey) {
				t.Fatalf("invalid masked KeyHint: %q", hint)
			}
			if err := capture.Close(); err != nil {
				t.Fatal(err)
			}
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !enabled {
				if len(files) != 0 {
					t.Fatal("disabled capture wrote files")
				}
				return
			}
			phases := map[string]int{}
			for _, file := range files {
				data, err := os.ReadFile(filepath.Join(dir, file.Name()))
				if err != nil {
					t.Fatal(err)
				}
				for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					var entry debug.CaptureEntry
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						t.Fatal(err)
					}
					phases[entry.Phase]++
					if entry.Provider != "workbuddy" {
						t.Fatalf("capture provider=%q", entry.Provider)
					}
					if entry.Phase == debug.PhaseUpstreamResponse && entry.Data != wire {
						t.Fatal("capture did not persist the complete raw SSE")
					}
				}
			}
			if phases[debug.PhaseUpstreamRequest] != 1 || phases[debug.PhaseUpstreamResponse] != 1 {
				t.Fatalf("captured phases=%v", phases)
			}
		})
	}
}

func TestWorkBuddyExecuteReportsKeyHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1}}`)
	}))
	defer srv.Close()

	p, cfg := newWorkBuddyTestProvider(t)
	cfg.WorkBuddy.BaseURL = srv.URL
	cfg.WorkBuddy.APIKey = "k"
	p.atomic.ApplyLoaded(cfg)

	res, err := p.Execute(context.Background(), workBuddyTestRequest(),
		config.ModelConfig{Provider: "workbuddy", ModelID: "global:x"})
	if err != nil {
		t.Fatal(err)
	}
	if res.KeyHint == "" {
		t.Error("Execute returned no KeyHint")
	}
}
