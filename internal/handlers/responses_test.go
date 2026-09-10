package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/middleware"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/internal/token"
	"github.com/routatic/proxy/pkg/types"
)

func newResponsesTestHandler(t *testing.T, upstream http.Handler) (*MessagesHandler, *storage.Database) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if os.Getenv("TIKTOKEN_CACHE_DIR") == "" {
		t.Setenv("TIKTOKEN_CACHE_DIR", filepath.Join(home, "tiktoken"))
	}
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	cfg := config.NewAtomicConfig(&config.Config{
		CommandCode:           config.CommandCodeConfig{APIKey: "synthetic-test-key", BaseURL: server.URL + "/chat/completions", AnthropicBaseURL: server.URL + "/messages"},
		Models:                map[string]config.ModelConfig{"default": {Provider: "commandcode", ModelID: "synthetic-model", MaxTokens: 8192, Vision: true}},
		RespectRequestedModel: boolPtr(false),
	}, "")
	registry := core.NewProviderRegistry()
	if err := registry.Register(provider.NewCommandCodeProvider(cfg, nil)); err != nil {
		t.Fatal(err)
	}
	db, err := storage.Open(storage.Config{DatabasePath: filepath.Join(home, "test.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	counter, err := token.NewCounter()
	if err != nil {
		t.Fatal(err)
	}
	h := NewMessagesHandler(client.NewOpenCodeClient(cfg, nil), registry, router.NewModelRouter(cfg),
		router.NewFallbackHandler(slog.Default(), 3, time.Second),
		counter, metrics.New(), nil, NewStorageAdapter(db))
	return h, db
}

func TestHandleResponsesNonStreamingUsesSharedRoutingAndAccounting(t *testing.T) {
	var got types.ChatCompletionRequest
	h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer synthetic-test-key" {
			t.Error("request did not use the dedicated CommandCode endpoint/auth")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chat_test","choices":[{"message":{"role":"assistant","content":"answer","tool_calls":[{"id":"call_new","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"next\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":101,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":90}}}`)
	}))
	h.rateLimiter = middleware.NewRateLimiter(1, time.Minute)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model":"requested-model","instructions":"be precise","store":false,"prompt_cache_key":"synthetic-session","client_metadata":{"originator":"synthetic-client"},
		"parallel_tool_calls":false,"tool_choice":"required","reasoning":{"effort":"high"},"max_output_tokens":1234,
		"input":[{"role":"user","content":"hello"},{"type":"function_call","call_id":"call_old","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_old","output":"prior result"}],
		"tools":[{"type":"function","name":"lookup","strict":false,"parameters":{"type":"object","properties":{}}}]}`))
	req.Header.Set("X-Request-ID", "synthetic-responses-request")
	w := httptest.NewRecorder()
	h.HandleResponses(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("X-Request-ID") != "synthetic-responses-request" {
		t.Fatalf("response status/headers = %d %v; body=%s", w.Code, w.Header(), w.Body.String())
	}
	if got.Model != "synthetic-model" || len(got.Messages) != 4 || got.Messages[3].Role != "tool" || got.Messages[3].ToolCallID != "call_old" || got.Messages[3].ContentText() != "prior result" {
		t.Fatalf("upstream conversation = %+v", got)
	}
	encoded, _ := json.Marshal(got)
	if !strings.Contains(string(encoded), `"parallel_tool_calls":false`) || !strings.Contains(string(encoded), `"tool_choice":"required"`) || !strings.Contains(string(encoded), `"reasoning_effort":"high"`) {
		t.Fatalf("upstream constraints lost: %s", encoded)
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response["object"] != "response" || response["status"] != "completed" {
		t.Fatalf("body=%s err=%v", w.Body.String(), err)
	}
	if !strings.Contains(w.Body.String(), `"call_id":"call_new"`) || !strings.Contains(w.Body.String(), `"cached_tokens":90`) {
		t.Fatalf("response lost function/usage: %s", w.Body.String())
	}
	rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("accounting rows=%+v total=%d err=%v", rows, total, err)
	}
	row := rows[0]
	if row.Provider != "commandcode" || row.Model != "synthetic-model" || row.InputTokens != 11 || row.CacheReadTokens != 90 || row.OutputTokens != 7 || !row.Success || row.Streaming {
		t.Fatalf("shared accounting changed: %+v", row)
	}
}

func TestHandleResponsesStreamsBeforeUpstreamCompletion(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	allowFinish := func() { once.Do(func() { close(release) }) }
	t.Cleanup(allowFinish)
	h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":101,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":90}}}\n\ndata: [DONE]\n\n")
	}))
	downstream := httptest.NewServer(http.HandlerFunc(h.HandleResponses))
	t.Cleanup(downstream.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+"/v1/responses", strings.NewReader(`{"model":"m","input":"hi","store":false,"stream":true}`))
	resp, err := downstream.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream headers=%v status=%d", resp.Header, resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	var body strings.Builder
	sawDelta := false
	for scanner.Scan() {
		line := scanner.Text()
		body.WriteString(line)
		body.WriteByte('\n')
		if strings.HasPrefix(line, "data:") && strings.Contains(line, `"type":"response.output_text.delta"`) {
			sawDelta = true
			allowFinish()
		}
	}
	if scanner.Err() != nil || !sawDelta || !strings.Contains(body.String(), "response.completed") || !strings.Contains(body.String(), `"cached_tokens":90`) {
		t.Fatalf("not a real complete stream: err=%v body=%s", scanner.Err(), body.String())
	}
	rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 1 || rows[0].Provider != "commandcode" || rows[0].InputTokens != 11 || rows[0].CacheReadTokens != 90 || !rows[0].Streaming {
		t.Fatalf("stream accounting rows=%+v total=%d err=%v", rows, total, err)
	}
}

func TestHandleResponsesRejectsUnsupportedFeaturesBeforeRouting(t *testing.T) {
	h := NewMessagesHandler(nil, nil, nil, nil, nil, metrics.New(), nil, nil)
	for _, body := range []string{
		`{"model":"m","input":"hi","tools":[{"type":"custom","name":"apply_patch"}]}`,
		`{"model":"m","input":"hi","tools":[{"type":"namespace","name":"multi_agent_v1"}]}`,
		`{"model":"m","input":"hi","previous_response_id":"resp_old"}`,
		`{"model":"m","input":[],"store":true}`,
		`{"model":"m","input":[{"type":"function_call_output","call_id":"c","output":[{"type":"input_image","image_url":"https://example.invalid/a.png"}]}]}`,
		`{"model":"m","input":"hi"} {"model":"other"}`,
	} {
		w := httptest.NewRecorder()
		h.HandleResponses(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"error"`) || strings.Contains(w.Body.String(), `"object":"response"`) {
			t.Fatalf("unsupported request became success: code=%d body=%s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	h.HandleResponses(w, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET status=%d headers=%v", w.Code, w.Header())
	}
}

func TestHandleResponsesInvalidRequestsShareAdmissionLimit(t *testing.T) {
	h := NewMessagesHandler(nil, nil, nil, nil, nil, metrics.New(), nil, nil)
	h.rateLimiter = middleware.NewRateLimiter(1, time.Minute)
	w := httptest.NewRecorder()
	h.HandleResponses(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"unsupported":true}`)))
	if w.Code != http.StatusBadRequest || w.Header().Get("X-Request-ID") == "" {
		t.Fatalf("first request status=%d headers=%v", w.Code, w.Header())
	}
	w = httptest.NewRecorder()
	h.HandleMessages(w, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`)))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("malformed Responses request bypassed shared limit: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.HandleResponses(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`)))
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), `"type":"rate_limit_error"`) {
		t.Fatalf("rate-limited Responses result: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestResponsesHTTPFinishIsIdempotent(t *testing.T) {
	for _, body := range []string{
		`{"type":"message","model":"m","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`,
		`{"type":"message","model":"m","content":[{"type":"unsupported"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`,
	} {
		w := httptest.NewRecorder()
		bridge := &responsesHTTPWriter{ResponseWriter: w, headers: make(http.Header), model: "m"}
		bridge.WriteHeader(http.StatusOK)
		if _, err := bridge.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
		firstErr := bridge.Finish()
		firstBody := w.Body.String()
		secondErr := bridge.Finish()
		if firstErr != secondErr || w.Body.String() != firstBody {
			t.Fatalf("Finish wrote twice or changed error: first=%v second=%v body=%s", firstErr, secondErr, w.Body.String())
		}
	}
}

func TestHandleResponsesTruncatedNativeStreamPreservesFailedUsage(t *testing.T) {
	h, db := newResponsesTestHandler(t, http.NotFoundHandler())
	h.providerRegistry = core.NewProviderRegistry()
	provider := &fragmentedAnthropicProvider{usageLimitStreamProvider: usageLimitStreamProvider{
		name: "commandcode",
		body: "data: {\"type\":\"message_start\",\"message\":{\"model\":\"synthetic-model\",\"usage\":{\"input_tokens\":11,\"cache_read_input_tokens\":90}}}\n\n" +
			"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n" +
			"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n",
	}}
	if err := h.providerRegistry.Register(provider); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.HandleResponses(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","stream":true}`)))
	if !strings.Contains(w.Body.String(), "response.failed") || strings.Contains(w.Body.String(), "response.completed") {
		t.Fatalf("truncated stream reported success: %s", w.Body.String())
	}
	rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 1 || rows[0].Success || rows[0].InputTokens != 11 || rows[0].OutputTokens != 7 || rows[0].CacheReadTokens != 90 {
		t.Fatalf("truncated stream accounting=%+v total=%d err=%v", rows, total, err)
	}
}

func TestHandleResponsesUpstreamFailureNeverCompletes(t *testing.T) {
	h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"synthetic auth rejection"}}`)
	}))
	for _, stream := range []string{"false", "true"} {
		w := httptest.NewRecorder()
		h.HandleResponses(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","stream":`+stream+`}`)))
		if stream == "false" && w.Code != http.StatusBadGateway {
			t.Fatalf("failure status=%d body=%s", w.Code, w.Body.String())
		}
		if stream == "true" && (!strings.Contains(w.Body.String(), "response.failed") || strings.Contains(w.Body.String(), "response.completed")) {
			t.Fatalf("failed upstream looks complete: %s", w.Body.String())
		}
	}
	_, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 0 {
		t.Fatalf("failed unauthenticated request billed: total=%d err=%v", total, err)
	}
}

func TestHandleResponsesCancellationReachesUpstream(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	h, _ := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","input":"hi","stream":true}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.HandleResponses(w, req)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream never received request")
	}
	cancel()
	for _, ch := range []chan struct{}{canceled, done} {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("cancellation was not propagated")
		}
	}
	if strings.Contains(w.Body.String(), "response.completed") {
		t.Fatalf("canceled request completed: %s", w.Body.String())
	}
}

// Run explicitly with ROUTATIC_CODEX_SMOKE=1. Ordinary tests neither require an
// installed CLI nor load its user configuration or contact a real model API.
func TestCodexResponsesCLIToolRoundTrip(t *testing.T) {
	if os.Getenv("ROUTATIC_CODEX_SMOKE") != "1" {
		t.Skip("manual CLI smoke: set ROUTATIC_CODEX_SMOKE=1")
	}
	cli, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	var returnedToolOutput atomic.Bool
	h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request types.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid synthetic request", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-test-key" || request.Model != "synthetic-model" || request.Stream == nil || !*request.Stream {
			t.Error("CLI request did not use the configured CommandCode model/auth/stream")
			http.Error(w, "unexpected synthetic request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(value any) {
			data, err := json.Marshal(value)
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		}
		finish := "stop"
		switch requests.Add(1) {
		case 1:
			found := false
			for _, tool := range request.Tools {
				found = found || tool.Function.Name == "exec_command"
			}
			if !found {
				t.Error("CLI did not advertise exec_command")
				http.Error(w, "missing exec_command", http.StatusBadRequest)
				return
			}
			const arguments = `{"cmd":"printf CODEX_TOOL_OK"}`
			for i, fragment := range []string{arguments[:16], arguments[16:]} {
				function := map[string]any{"arguments": fragment}
				call := map[string]any{"index": 0, "function": function}
				if i == 0 {
					function["name"] = "exec_command"
					call["id"], call["type"] = "call_synthetic_exec", "function"
				}
				emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{call}}}}})
			}
			finish = "tool_calls"
		case 2:
			for _, message := range request.Messages {
				if message.Role == "tool" && message.ToolCallID == "call_synthetic_exec" && strings.Contains(message.ContentText(), "CODEX_TOOL_OK") {
					returnedToolOutput.Store(true)
				}
			}
			if !returnedToolOutput.Load() {
				t.Error("CLI tool output did not return through the adapter")
			}
			emit(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "CODEX_SMOKE_OK"}}}})
		default:
			t.Error("CLI made more than the two expected inference requests")
			return
		}
		emit(map[string]any{
			"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}},
			"usage":   map[string]any{"prompt_tokens": 101, "completion_tokens": 7, "prompt_tokens_details": map[string]int{"cached_tokens": 90}},
		})
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	downstream := httptest.NewServer(http.HandlerFunc(h.HandleResponses))
	t.Cleanup(downstream.Close)
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	args := []string{"exec", "--ephemeral", "--ignore-user-config", "--ignore-rules", "--skip-git-repo-check", "--sandbox", "read-only", "--json", "--model", "commandcode"}
	for _, setting := range []string{
		`model_provider="smoke"`, `model_providers.smoke.name="Synthetic local test"`,
		`model_providers.smoke.base_url="` + downstream.URL + `/v1"`,
		`model_providers.smoke.wire_api="responses"`, `model_providers.smoke.env_key="SYNTHETIC_PROXY_KEY"`,
		`model_providers.smoke.request_max_retries=0`, `model_providers.smoke.stream_max_retries=0`,
		`web_search="disabled"`, `model_supports_reasoning_summaries=false`, `model_reasoning_summary="none"`,
		`features.multi_agent=false`, `features.remote_plugin=false`, `features.apps=false`, `features.shell_snapshot=false`,
	} {
		args = append(args, "-c", setting)
	}
	args = append(args, "Run exactly printf CODEX_TOOL_OK with exec_command, then report the result. Do not read or change any files.")
	cmd := exec.CommandContext(ctx, cli, args...)
	cmd.Dir = home
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "CODEX_HOME=" + home, "SYNTHETIC_PROXY_KEY=synthetic-client-key", "TERM=dumb"}
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "CODEX_SMOKE_OK") || requests.Load() != 2 || !returnedToolOutput.Load() {
		t.Fatalf("CLI smoke failed: err=%v requests=%d tool_output_returned=%v\n%s", err, requests.Load(), returnedToolOutput.Load(), output)
	}
	// Wait for the HTTP handler to finish its accounting after the terminal SSE.
	downstream.Close()
	rows, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("CLI smoke accounting rows=%+v total=%d err=%v", rows, total, err)
	}
	for _, row := range rows {
		if row.Provider != "commandcode" || !row.Success || !row.Streaming || row.InputTokens != 11 || row.CacheReadTokens != 90 || row.OutputTokens != 7 {
			t.Fatalf("CLI smoke accounting changed: %+v", row)
		}
	}
	t.Logf("CLI=%s; commandcode alias; two inference requests; printf tool result round-tripped; final CODEX_SMOKE_OK; two isolated commandcode accounting records", cli)
}
