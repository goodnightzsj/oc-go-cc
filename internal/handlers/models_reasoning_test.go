package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/site"
)

func TestModelListingPublishesWorkBuddyReasoningFacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[
			{"id":"global:known","reasoning_supported_efforts":["low","high","max","off"],"reasoning_default_effort":"max","can_disable_thinking":true},
			{"id":"cn:fixed","reasoning_supported_efforts":["medium"],"can_disable_thinking":false},
			{"id":"global:unknown"}
		]}`)
	}))
	defer srv.Close()
	cfg := &config.Config{
		ActiveSite: site.WorkBuddy,
		WorkBuddy:  config.WorkBuddyConfig{BaseURL: srv.URL, APIKey: "synthetic-key"},
		Models: map[string]config.ModelConfig{
			"alias": {Provider: site.WorkBuddy, ModelID: "global:known"},
			// Operator overrides affect emitted controls, not the platform facts
			// published for its native model ID.
			"global:known": {Provider: site.WorkBuddy, ModelID: "global:known", Reasoning: &config.ReasoningConfig{Levels: []string{"high"}}},
		},
	}
	h := NewModelsHandler(router.NewModelRouter(config.NewAtomicConfig(cfg, "")))
	w := httptest.NewRecorder()
	h.HandleListModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	var response struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Data) != 4 {
		t.Fatalf("unexpected listing: %d %s", w.Code, w.Body.String())
	}
	for _, entry := range response.Data {
		var id string
		_ = json.Unmarshal(entry["id"], &id)
		switch id {
		case "global:known":
			var levels []string
			_ = json.Unmarshal(entry["reasoning_supported_efforts"], &levels)
			if !reflect.DeepEqual(levels, []string{"low", "high", "max", "off"}) || string(entry["reasoning_default_effort"]) != `"max"` || string(entry["can_disable_thinking"]) != "true" {
				t.Fatalf("known model lost upstream facts: %v", entry)
			}
		case "cn:fixed":
			if string(entry["can_disable_thinking"]) != "false" || entry["reasoning_default_effort"] != nil {
				t.Fatalf("explicit false/default omission changed: %v", entry)
			}
		default:
			if entry["reasoning_supported_efforts"] != nil || entry["reasoning_default_effort"] != nil || entry["can_disable_thinking"] != nil {
				t.Fatalf("unknown/alias received invented directory facts: %v", entry)
			}
		}
	}
}

func TestModelListingDoesNotPublishAnotherPlatformsReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[{"id":"example","reasoning_supported_efforts":["high"],"reasoning_default_effort":"high","can_disable_thinking":true}]}`)
	}))
	defer srv.Close()
	cfg := &config.Config{ActiveSite: site.CommandCode, CommandCode: config.CommandCodeConfig{BaseURL: srv.URL + "/chat/completions", APIKey: "synthetic-key"}}
	h := NewModelsHandler(router.NewModelRouter(config.NewAtomicConfig(cfg, "")))
	w := httptest.NewRecorder()
	h.HandleListModels(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	var response openAIModelList
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Data) != 1 {
		t.Fatalf("unexpected listing: %d %s", w.Code, w.Body.String())
	}
	model := response.Data[0]
	if model.ReasoningSupportedEfforts != nil || model.ReasoningDefaultEffort != "" || model.CanDisableThinking != nil {
		t.Fatalf("WorkBuddy metadata affected CommandCode: %+v", model)
	}
}
