package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

// The capability filter is per model, and the gateway's response to a wrong
// value is a hard refusal - not a fallback. A model configured with `has` must
// therefore carry exactly its own list, and one configured without it must
// carry none at all, or the two models would fail each other's requests.
func TestClinePassCapabilitiesAreInjectedPerModel(t *testing.T) {
	var got map[string]json.RawMessage
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		got = nil
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})

	cfg := *p.atomic.Get()
	cfg.ClinePass.ChannelPinEnabled = true
	cfg.ClinePass.ChannelPin = "deepseek"
	cfg.ClinePass.ChannelPinCapabilities = map[string][]string{
		"cline-pass/deepseek-v4.1-flash-1017": {"vision", "implicit-caching"},
		"cline-pass/deepseek-v4-pro":          {"implicit-caching"},
	}
	p.atomic.ApplyLoaded(&cfg)

	call := func(modelID string) {
		t.Helper()
		model := config.ModelConfig{Provider: "cline-pass", ModelID: modelID}
		body, err := p.Stream(context.Background(), clinePassRequest(), model)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}

	// Model with two capabilities: both pipelines carry exactly those two.
	call("cline-pass/deepseek-v4.1-flash-1017")
	var options struct {
		Only []string `json:"only"`
		Has  []string `json:"has"`
	}
	if raw, ok := got["providerOptions"]; ok {
		var wrapper struct {
			Gateway json.RawMessage `json:"gateway"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(wrapper.Gateway, &options); err != nil {
			t.Fatal(err)
		}
		if len(options.Has) != 2 || options.Has[0] != "vision" || options.Has[1] != "implicit-caching" {
			t.Errorf("planner has = %v, want [vision implicit-caching]", options.Has)
		}
	} else {
		t.Fatal("pinned request carried no providerOptions")
	}
	if raw, ok := got["provider"]; ok {
		var direct struct {
			Has []string `json:"has"`
		}
		if err := json.Unmarshal(raw, &direct); err != nil {
			t.Fatal(err)
		}
		if len(direct.Has) != 2 {
			t.Errorf("direct has = %v, want the same two capabilities", direct.Has)
		}
	} else {
		t.Fatal("pinned request carried no provider")
	}

	// The other model has one capability. Its request must not inherit the
	// first model's list: vision would be refused for it.
	call("cline-pass/deepseek-v4-pro")
	var fresh struct {
		Gateway struct {
			Has []string `json:"has"`
		} `json:"gateway"`
	}
	if err := json.Unmarshal(got["providerOptions"], &fresh); err != nil {
		t.Fatal(err)
	}
	if len(fresh.Gateway.Has) != 1 || fresh.Gateway.Has[0] != "implicit-caching" {
		t.Errorf("second model has = %v, want [implicit-caching]", fresh.Gateway.Has)
	}
}

// A model with no configured capabilities must send no `has` field at all.
// `"has": []` and a missing field are not the same request, and only the
// missing one is the shape every existing deployment already sends.
func TestClinePassOmitsCapabilitiesForUnconfiguredModel(t *testing.T) {
	var raw json.RawMessage
	p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw = nil
		body, _ := io.ReadAll(r.Body)
		raw = body
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})

	cfg := *p.atomic.Get()
	cfg.ClinePass.ChannelPinEnabled = true
	cfg.ClinePass.ChannelPin = "deepseek"
	cfg.ClinePass.ChannelPinCapabilities = map[string][]string{
		"some-other-model": {"vision"},
	}
	p.atomic.ApplyLoaded(&cfg)

	model := config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/unlisted-model"}
	body, err := p.Stream(context.Background(), clinePassRequest(), model)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()

	if containsField(raw, "has") {
		t.Errorf("unconfigured model sent a capability filter: %s", string(raw))
	}
	// Pinning itself must still be there: omitting the filter must not disable it.
	if !containsField(raw, "only") {
		t.Error("pinning was lost when the capability filter was absent")
	}
}

func containsField(raw []byte, field string) bool {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return false
	}
	for _, value := range top {
		var nested map[string]json.RawMessage
		if json.Unmarshal(value, &nested) == nil {
			if _, ok := nested[field]; ok {
				return true
			}
			for _, inner := range nested {
				var deeper map[string]json.RawMessage
				if json.Unmarshal(inner, &deeper) == nil {
					if _, ok := deeper[field]; ok {
						return true
					}
				}
			}
		}
	}
	return false
}
