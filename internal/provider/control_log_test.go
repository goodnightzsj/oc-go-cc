package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/core"
)

func TestRequestControlLogExcludesContentAndKeepsBody(t *testing.T) {
	const original = `{"temperature":0,"reasoning":{"effort":"none","summary":"auto"},"prompt_cache_key":"private-affinity","input":"private-prompt"}`
	const wire = `{"model":"m","messages":[{"role":"user","content":"private-prompt"}],"temperature":0.5,"reasoning_effort":"low","tools":[{"function":{"name":"private-tool","parameters":{"private-schema":{}}}}],"stop":["private-stop"],"thinking":{"type":"enabled","signature":"private-signature"}}`
	metadata := core.RequestMetadata{RequestID: "synthetic", ResponsesBody: json.RawMessage(original), TargetFormat: core.WireFormatOpenAIChat}
	req, err := http.NewRequestWithContext(core.WithRequestMetadata(t.Context(), metadata), http.MethodPost, "https://example.invalid", strings.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer private-credential")
	var log bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logRequestControls(req, logger)
	if strings.Contains(log.String(), "private-") {
		t.Fatalf("content leaked into control log: %s", log.String())
	}
	var entry struct {
		Source             string `json:"source_format"`
		Target             string `json:"target_format"`
		Requested, Emitted map[string]any
		Verified           bool `json:"effect_verified"`
	}
	if err := json.Unmarshal(log.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Source != "responses" || entry.Target != "openai" || entry.Verified || entry.Requested["temperature"] != float64(0) || entry.Requested["reasoning_effort"] != "none" || entry.Emitted["reasoning_effort"] != "low" || entry.Emitted["stop_count"] != float64(1) || entry.Requested["prompt_cache_key_present"] != true || entry.Emitted["prompt_cache_key_present"] != false {
		t.Fatalf("control observation lost: %s", log.String())
	}
	body, err := io.ReadAll(req.Body)
	if err != nil || string(body) != wire {
		t.Fatalf("live body changed: %s err=%v", body, err)
	}

	req.GetBody = func() (io.ReadCloser, error) { t.Fatal("disabled debug must not read body"); return nil, nil }
	logRequestControls(req, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestControlSummaryProtocolFields(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want map[string]any
	}{
		{`{"max_tokens":100,"top_p":0.5,"thinking":{"type":"enabled","budget_tokens":0},"output_config":{"format":{"type":"json_schema"}},"tool_choice":{"type":"tool","name":"private","disable_parallel_tool_use":true},"stop_sequences":["a","b"]}`, map[string]any{"max_tokens": 100, "top_p": 0.5, "thinking_budget": 0, "thinking_mode": "enabled", "output_format": "json_schema", "parallel_tools": false, "tool_choice": "tool", "stop_count": 2}},
		{`{"max_output_tokens":80,"text":{"format":{"type":"json_schema"}},"parallel_tool_calls":true,"tool_choice":"required"}`, map[string]any{"max_tokens": 80, "output_format": "json_schema", "parallel_tools": true, "tool_choice": "required"}},
		{`{"generationConfig":{"maxOutputTokens":90,"temperature":0,"topP":0.3,"thinkingConfig":{"thinkingBudget":-1},"stopSequences":["a"],"responseMimeType":"application/json"},"toolConfig":{"functionCallingConfig":{"mode":"ANY"}}}`, map[string]any{"max_tokens": 90, "temperature": float64(0), "top_p": 0.3, "thinking_budget": -1, "stop_count": 1, "response_mime_type": "application/json", "tool_choice": "ANY"}},
	} {
		got, err := controlSummary([]byte(tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		for key, want := range tc.want {
			if got[key] != want {
				t.Errorf("%s=%v want=%v source=%s", key, got[key], want, tc.raw)
			}
		}
	}
}
