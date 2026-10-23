package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func newWorkBuddyTestProvider(t *testing.T) (*WorkBuddyProvider, *config.Config) {
	t.Helper()
	cfg := &config.Config{}
	return NewWorkBuddyProvider(config.NewAtomicConfig(cfg, ""), nil), cfg
}

func workBuddyTestRequest() *types.MessageRequest {
	return &types.MessageRequest{
		Model:     "workbuddy",
		MaxTokens: 64,
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
}

// The gateway names its models with a realm prefix (`global:...`). That prefix
// is part of the wire id, the same way `cline-pass/` is for ClinePass: stripping
// it sends a name the upstream does not recognise. This pins the pass-through so
// a future "tidy up the model id" change fails here instead of in production.
func TestWorkBuddySendsModelIDUnchanged(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = nil
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer srv.Close()

	p, cfg := newWorkBuddyTestProvider(t)
	cfg.WorkBuddy.BaseURL = srv.URL
	cfg.WorkBuddy.APIKey = "test-key"
	p.atomic.ApplyLoaded(cfg)

	for _, modelID := range []string{
		"global:deepseek-v4.1-flash",
		"cn:deepseek-v4.1-flash",
		"plain-model",
	} {
		model := config.ModelConfig{Provider: "workbuddy", ModelID: modelID}
		if _, err := p.Execute(context.Background(), workBuddyTestRequest(), model); err != nil {
			t.Fatalf("%s: %v", modelID, err)
		}
		if send, _ := got["model"].(string); send != modelID {
			t.Errorf("model sent = %q, want %q unchanged", send, modelID)
		}
	}
}

// The gateway is local and unauthenticated on the wire only if no key is set;
// this proxy always presents one, and a missing key must fail before a request
// is attempted rather than reaching the gateway and coming back as a 401.
func TestWorkBuddyRequiresAKey(t *testing.T) {
	p, cfg := newWorkBuddyTestProvider(t)
	cfg.WorkBuddy.BaseURL = "http://127.0.0.1:1" // never dialed
	cfg.WorkBuddy.APIKey = ""
	cfg.WorkBuddy.APIKeys = nil
	p.atomic.ApplyLoaded(cfg)

	model := config.ModelConfig{Provider: "workbuddy", ModelID: "global:x"}
	_, err := p.Execute(context.Background(), workBuddyTestRequest(), model)
	if err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("err = %v, want a missing-key error", err)
	}
}

// The stream flag is always written. A gateway defaulting it to true would turn
// every non-streaming call into an SSE body the caller cannot parse.
func TestWorkBuddyWritesStreamFlag(t *testing.T) {
	for _, stream := range []bool{false, true} {
		var got map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = nil
			_ = json.NewDecoder(r.Body).Decode(&got)
			if stream {
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				return
			}
			_, _ = io.WriteString(w, `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
		}))

		p, cfg := newWorkBuddyTestProvider(t)
		cfg.WorkBuddy.BaseURL = srv.URL
		cfg.WorkBuddy.APIKey = "k"
		p.atomic.ApplyLoaded(cfg)
		model := config.ModelConfig{Provider: "workbuddy", ModelID: "global:x"}

		var err error
		if stream {
			var body io.ReadCloser
			body, err = p.Stream(context.Background(), workBuddyTestRequest(), model)
			if err == nil {
				_, _ = io.Copy(io.Discard, body)
				_ = body.Close()
			}
		} else {
			_, err = p.Execute(context.Background(), workBuddyTestRequest(), model)
		}
		srv.Close()
		if err != nil {
			t.Fatalf("stream=%v: %v", stream, err)
		}
		if send, ok := got["stream"].(bool); !ok || send != stream {
			t.Errorf("stream=%v sent as %v", stream, got["stream"])
		}
	}
}
