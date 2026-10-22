package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

// upstreamNamesAnotherModel answers every request with a different model than
// the one asked for, which is what ClinePass's gateway does when it answers an
// id by running a virtual model behind its own fallback chain.
func upstreamNamesAnotherModel(model string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		// provider_metadata rides inside delta, which is where the real gateway
		// puts it - a fixture with it anywhere else would pass while the live
		// path recorded nothing.
		_, _ = io.WriteString(w, "data: {\"model\":\""+model+"\",\"choices\":[{\"delta\":{"+
			"\"content\":\"ok\",\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"deepseek\","+
			"\"canonicalSlug\":\""+model+"\"}}}}}]}\n\n"+
			"data: {\"model\":\""+model+"\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],"+
			"\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n\n"+
			"data: [DONE]\n\n")
	}
}

// A response must name the model that ran, not the one that was requested.
// Everything downstream of this proxy reads that field - the dashboard, the
// client, and any gateway sitting in front of it - so leaving the request id
// there tells all of them a model executed that did not.
func TestNonStreamingResponseNamesTheServedModel(t *testing.T) {
	const served = "vmc/deepseek-v4p1-flash-1017-contributor-fallbacks"
	p, _ := clinePassTestProvider(t, upstreamNamesAnotherModel(served))

	result, err := p.Execute(context.Background(), clinePassRequest(),
		config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/deepseek-v4.1-flash-1017"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var response struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(result.Body, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Model != served {
		t.Errorf("response model = %q, want the served model %q", response.Model, served)
	}
}

// The streaming path cannot wait for the routing block at the end of the
// stream, so it uses the model the first chunk names. That has to be available
// from the body the handler forwards, or message_start would fall back to the
// request id.
func TestStreamingBodyReportsTheModelFromTheFirstChunk(t *testing.T) {
	const served = "deepseek/deepseek-v4.1-flash-1017"
	p, _ := clinePassTestProvider(t, upstreamNamesAnotherModel(served))

	body, err := p.Stream(context.Background(), clinePassRequest(),
		config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/deepseek-v4.1-flash-1017"})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer func() { _ = body.Close() }()

	// Available before the caller reads anything: the provider peeked.
	if got := client.ServedModelOf(body); got != served {
		t.Fatalf("served model = %q, want %q before the body is drained", got, served)
	}

	// And the peek must not have swallowed the frame it read.
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if !strings.Contains(string(raw), "\"content\":\"ok\"") {
		t.Errorf("the peeked frame was lost: body = %q", string(raw))
	}
	if !strings.Contains(string(raw), "data: [DONE]") {
		t.Errorf("the stream was truncated: body = %q", string(raw))
	}
}
