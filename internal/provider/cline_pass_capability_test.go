package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

// Legacy has values remain configurable but cannot establish a channel contract.
func TestClinePassAutomaticRoutingOmitsLegacyCapabilities(t *testing.T) {
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		if got["provider"] != nil || got["providerOptions"] != nil {
			t.Error("legacy has leaked into automatic routing")
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	cfg := *p.atomic.Get()
	cfg.ClinePass.ChannelPinCapabilities = map[string][]string{"synthetic": {"vision", "implicit-caching"}}
	p.atomic.ApplyLoaded(&cfg)
	for _, id := range []string{"synthetic", "unconfigured"} {
		body, err := p.Stream(t.Context(), clinePassRequest(), config.ModelConfig{Provider: p.Name(), ModelID: id})
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(io.Discard, body)
		_ = body.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(p.atomic.Get().ClinePass.ChannelPinCapabilities["synthetic"]) != 2 {
		t.Fatal("legacy configuration was destroyed")
	}
}
