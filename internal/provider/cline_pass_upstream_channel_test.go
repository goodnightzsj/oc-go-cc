package provider

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

// clineChannelStream is the tail of a real Cline response: content first, then
// the routing metadata in the last frame before [DONE]. The order matters - it
// is why the channel can only be read after the stream ends.
const clineChannelStream = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"openai-compatible-private\"}}}}}]}\n\n" +
	"data: [DONE]\n\n"

func channelUpstream(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, clineChannelStream)
	}
}

// The observed channel must reach the caller from both request shapes. It is
// read after the body is drained because the gateway names it in the last frame
// - reading it earlier yields "" for every request, which looks like a platform
// that does not report a channel at all.
func TestClinePassReportsTheServingChannel(t *testing.T) {
	for _, pinEnabled := range []bool{true, false} {
		name := "pinning off"
		if pinEnabled {
			name = "pinning on"
		}
		t.Run(name, func(t *testing.T) {
			p, _ := clinePassTestProvider(t, channelUpstream(t))
			cfg := p.atomic.Get()
			cfg.ClinePass.ChannelPinEnabled = pinEnabled
			cfg.ClinePass.ChannelPin = "deepseek"
			model := config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/deepseek-v4.1-flash"}

			t.Run("Execute", func(t *testing.T) {
				result, err := p.Execute(context.Background(), clinePassRequest(), model)
				if err != nil {
					t.Fatal(err)
				}
				if result.UpstreamChannel != "openai-compatible-private" {
					t.Errorf("Execute channel = %q, want the gateway's reported channel", result.UpstreamChannel)
				}
			})

			t.Run("Stream", func(t *testing.T) {
				body, err := p.Stream(context.Background(), clinePassRequest(), model)
				if err != nil {
					t.Fatal(err)
				}
				// Before draining: the metadata frame has not been read, so the
				// honest answer is "not known yet".
				// Stream reads ahead to the model frame, so the routing metadata
				// on it is observed before the body is returned.
				if got := client.UpstreamChannelOf(body); got != "openai-compatible-private" {
					t.Errorf("channel after the peek = %q, want the gateway's reported channel", got)
				}
				if _, err := io.Copy(io.Discard, body); err != nil {
					t.Fatal(err)
				}
				if got := client.UpstreamChannelOf(body); got != "openai-compatible-private" {
					t.Errorf("channel after draining = %q, want the gateway's reported channel", got)
				}
				_ = body.Close()
			})
		})
	}
}

// A response with no routing metadata must report no channel rather than a
// guess. Not every model publishes one - glm models were observed returning no
// provider_metadata at all.
func TestClinePassReportsNoChannelWhenTheGatewayNamesNone(t *testing.T) {
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	model := config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/glm-5.3-flash"}

	result, err := p.Execute(context.Background(), clinePassRequest(), model)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpstreamChannel != "" {
		t.Errorf("channel = %q, want empty when the gateway reports none", result.UpstreamChannel)
	}
}
