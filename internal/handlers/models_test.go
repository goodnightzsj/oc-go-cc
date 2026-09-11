package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/router"
)

func TestHandleListModels_ReturnsOpenAIEnvelope(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"default":   {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4-5-20250929": {Provider: "opencode-zen", ModelID: "minimax"},
		},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	handler := NewModelsHandler(router.NewModelRouter(atomic))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	handler.HandleListModels(recorder, req)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body: %s", got, want, recorder.Body.String())
	}

	var resp openAIModelList
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is invalid JSON: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object = %q, want \"list\"", resp.Object)
	}

	ids := make(map[string]openAIModel, len(resp.Data))
	for _, m := range resp.Data {
		if m.Object != "model" {
			t.Errorf("model %q object = %q, want \"model\"", m.ID, m.Object)
		}
		// name and display_name carry the same value so both OpenAI clients
		// (CC-Switch) and Claude Code gateway discovery see a label.
		if m.Name != m.DisplayName {
			t.Errorf("model %q: name %q != display_name %q", m.ID, m.Name, m.DisplayName)
		}
		ids[m.ID] = m
	}
	for _, want := range []string{"default", "kimi-k2.6", "claude-sonnet-4-5-20250929"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("expected model %q in listing", want)
		}
	}
}

func TestHandleListModels_RejectsNonGET(t *testing.T) {
	atomic := config.NewAtomicConfig(&config.Config{}, "/tmp/test-config.json")
	handler := NewModelsHandler(router.NewModelRouter(atomic))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	handler.HandleListModels(recorder, req)

	if got, want := recorder.Code, http.StatusMethodNotAllowed; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestHandleListModels_ProviderDisplayOrder(t *testing.T) {
	cfg := &config.Config{Models: map[string]config.ModelConfig{
		"a-bedrock": {Provider: "aws-bedrock", ModelID: "model"},
		"b-router":  {Provider: "openrouter", ModelID: "model"},
		"c-zen":     {Provider: "opencode-zen", ModelID: "model"},
		"d-command": {Provider: "commandcode", ModelID: "model"},
		"z-go":      {Provider: "opencode-go", ModelID: "model"},
	}}
	handler := NewModelsHandler(router.NewModelRouter(config.NewAtomicConfig(cfg, "")))
	recorder := httptest.NewRecorder()
	handler.HandleListModels(recorder, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	var result openAIModelList
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	want := []string{"opencode-go", "commandcode", "opencode-zen", "aws-bedrock", "openrouter"}
	if len(result.Data) != len(want) {
		t.Fatalf("models = %d, want %d", len(result.Data), len(want))
	}
	for i, provider := range want {
		if result.Data[i].OwnedBy != provider {
			t.Fatalf("model %d provider = %q, want %q", i, result.Data[i].OwnedBy, provider)
		}
	}
}
