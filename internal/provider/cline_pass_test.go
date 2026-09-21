package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func clinePassTestProvider(t *testing.T, handler http.HandlerFunc) (*ClinePassProvider, *httptest.Server) {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	cfg := &config.Config{ClinePass: config.ClinePassConfig{
		APIKey:  "cline-key",
		BaseURL: upstream.URL + "/api/v1/chat/completions",
	}}
	return NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil), upstream
}

func clinePassRequest() *types.MessageRequest {
	return &types.MessageRequest{
		Model:     "cline-pass",
		MaxTokens: 4096,
		Messages:  []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}
}

// The platform has no Messages endpoint, so no model id may select one - not
// even a claude-* id, which is what would pick Anthropic on CommandCode.
func TestClinePassAlwaysUsesChatCompletions(t *testing.T) {
	p, _ := clinePassTestProvider(t, func(http.ResponseWriter, *http.Request) {})
	for _, modelID := range []string{"cline-pass/glm-5.3", "cline-pass/kimi-k3", "claude-sonnet-4-6", "cline-pass/deepseek-v4-pro"} {
		if got := core.ModelWireFormat(p, config.ModelConfig{Provider: "cline-pass", ModelID: modelID}).String(); got != "openai" {
			t.Errorf("%s = %s, want openai", modelID, got)
		}
	}
}

// The upstream flag is not the client's flag. The Cline API does not reliably
// serve stream:false, so a non-streaming call still asks upstream to stream -
// but it must return a complete JSON response, never SSE.
func TestClinePassNonStreamingForcesUpstreamStream(t *testing.T) {
	var upstreamStream string
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		upstreamStream = string(body["stream"])
		_, _ = io.WriteString(w, "data: {\"id\":\"gen-1\",\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"gen-1\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	result, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	if upstreamStream != "true" {
		t.Errorf("upstream saw stream=%s, want true: stream:false is not reliably served", upstreamStream)
	}
	// The caller gets a parsed document, not the raw SSE.
	for _, bad := range []string{"data:", "[DONE]"} {
		if strings.Contains(string(result.Body), bad) {
			t.Errorf("non-streaming result still contains SSE %q: %s", bad, result.Body)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(result.Body, &decoded); err != nil {
		t.Fatalf("result is not a JSON document: %v", err)
	}
}

// Streaming keeps the upstream flag a client asked for.
func TestClinePassStreamingSendsStreamTrue(t *testing.T) {
	var upstreamStream string
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		upstreamStream = string(body["stream"])
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
	})
	body, err := p.Stream(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	_, _ = io.Copy(io.Discard, body)
	if upstreamStream != "true" {
		t.Errorf("upstream saw stream=%s, want true", upstreamStream)
	}
}

// A gateway rejection can carry a bare HTML body. Reporting the empty string
// would lose the only diagnostic an operator has for that case.
func TestClinePassReportsNonJSONErrors(t *testing.T) {
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "<html><body>429 Too Many Requests</body></html>")
	})
	_, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/glm-5.3"})
	if err == nil {
		t.Fatal("a 429 was not reported as an error")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("error is %T, want *client.APIError", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", apiErr.StatusCode)
	}
	for _, want := range []string{"429", "text/html", "non-JSON"} {
		if !strings.Contains(apiErr.Body, want) {
			t.Errorf("error body %q does not mention %q", apiErr.Body, want)
		}
	}

	// A JSON error must pass through unmodified so nothing downstream re-parses
	// a rewritten body.
	p2, _ := clinePassTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(w, `{"error":{"code":402,"message":"Insufficient credits"}}`)
	})
	_, err = p2.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/glm-5.3"})
	apiErr, ok = err.(*client.APIError)
	if !ok {
		t.Fatalf("JSON error is %T, want *client.APIError", err)
	}
	if !strings.Contains(apiErr.Body, "Insufficient credits") || strings.Contains(apiErr.Body, "non-JSON") {
		t.Errorf("JSON error body was rewritten: %q", apiErr.Body)
	}
}

// The model id is sent to the upstream unchanged. The cline-pass/ prefix is the
// pool's own namespace and the upstream model slug; stripping it would ask for
// a model the gateway does not publish.
func TestClinePassForwardsModelIDUnchanged(t *testing.T) {
	var got string
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = body.Model
		_, _ = io.WriteString(w, "data: {\"id\":\"gen-1\",\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	if _, err := p.Execute(context.Background(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/qwen3.7-max"}); err != nil {
		t.Fatal(err)
	}
	if got != "cline-pass/qwen3.7-max" {
		t.Errorf("upstream model = %q, want the id unchanged", got)
	}
}
