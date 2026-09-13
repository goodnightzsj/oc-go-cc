package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

// A client learns what a platform offers by asking for the model list, so a
// platform the catalog does not know must still publish its own. CommandCode is
// absent from models.dev, which left its picker empty.
func TestHandleListModelsIncludesPlatformPublishedModels(t *testing.T) {
	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/provider/v1/models" {
			t.Errorf("model list requested from %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("x-api-key") != "" {
			t.Error("the published model list must not carry a credential")
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"deepseek/deepseek-v4-flash","name":"DeepSeek V4 Flash"},{"id":"zai-org/GLM-5.2","name":"GLM-5.2"}]}`)
	}))
	defer upstream.Close()

	list := func(t *testing.T, cfg *config.Config) map[string]openAIModel {
		t.Helper()
		handler := NewModelsHandler(router.NewModelRouter(config.NewAtomicConfig(cfg, "/tmp/test-config.json")))
		recorder := httptest.NewRecorder()
		handler.HandleListModels(recorder, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var resp openAIModelList
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		out := make(map[string]openAIModel, len(resp.Data))
		for _, m := range resp.Data {
			out[m.ID] = m
		}
		return out
	}

	configured := &config.Config{
		Models: map[string]config.ModelConfig{"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"}},
		CommandCode: config.CommandCodeConfig{
			APIKey:  "synthetic-key",
			BaseURL: upstream.URL + "/provider/v1/chat/completions",
		},
	}
	models := list(t, configured)
	if got := models["deepseek/deepseek-v4-flash"]; got.ID == "" || got.OwnedBy != "commandcode" {
		t.Fatalf("platform model missing from the listing: %+v", got)
	}
	if _, ok := models["kimi-k2.6"]; !ok {
		t.Fatal("configured models must stay in the listing alongside platform ones")
	}
	if hits.Load() == 0 {
		t.Fatal("the platform was never asked for its model list")
	}

	// Without a credential the platform is not asked at all: its models would
	// be unusable, and the request would advertise a platform this deployment
	// cannot call.
	hits.Store(0)
	unconfigured := &config.Config{
		Models:      configured.Models,
		CommandCode: config.CommandCodeConfig{BaseURL: configured.CommandCode.BaseURL},
	}
	if _, ok := list(t, unconfigured)["deepseek/deepseek-v4-flash"]; ok {
		t.Fatal("an unconfigured platform contributed models")
	}
	if hits.Load() != 0 {
		t.Fatalf("an unconfigured platform was queried %d times", hits.Load())
	}

	// An unreachable platform must not empty the rest of the listing.
	hits.Store(0)
	upstream.Close()
	if _, ok := list(t, configured)["kimi-k2.6"]; !ok {
		t.Fatal("one unreachable platform dropped every other model")
	}
}
