package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

func TestNativeResponsesReplayUsesCheckedSourceAcrossReload(t *testing.T) {
	for name, factory := range map[string]func(*config.AtomicConfig) core.Provider{
		"opencode-go":  func(c *config.AtomicConfig) core.Provider { return NewOpenCodeGoProvider(c, nil) },
		"opencode-zen": func(c *config.AtomicConfig) core.Provider { return NewOpenCodeZenProvider(c) },
		"aws-bedrock":  func(c *config.AtomicConfig) core.Provider { return NewAWSBedrockProvider(c) },
		"commandcode":  func(c *config.AtomicConfig) core.Provider { return NewCommandCodeProvider(c, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer synthetic-original" {
					t.Error("replay changed credential")
				}
				if name == "aws-bedrock" && r.Header.Get("OpenAI-Project") != "original-project" {
					t.Error("replay changed project")
				}
				if name == "commandcode" && r.Header.Get("x-cmd-zdr") != "1" {
					t.Error("replay changed retention")
				}
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, `{"status":"completed","output":[]}`)
			}))
			defer source.Close()
			original := &config.Config{
				OpenCodeGo:  config.OpenCodeGoConfig{APIKey: "synthetic-original", ResponsesBaseURL: source.URL},
				OpenCodeZen: config.OpenCodeZenConfig{APIKey: "synthetic-original", ResponsesBaseURL: source.URL},
				AWSBedrock:  config.AWSBedrockConfig{APIKey: "synthetic-original", BaseURL: source.URL, ProjectID: "original-project"},
				CommandCode: config.CommandCodeConfig{APIKey: "synthetic-original", ResponsesBaseURL: source.URL, ZeroDataRetention: true},
			}
			atomic := config.NewAtomicConfig(original, "")
			p := factory(atomic)
			ctx := core.WithRequestMetadata(context.Background(), core.RequestMetadata{ResponsesBody: []byte(`{"model":"synthetic","input":[{"type":"reasoning","encrypted_content":"synthetic","summary":[]}]}`), ResponsesReplay: true, ResponsesSource: original})
			atomic.ApplyLoaded(&config.Config{})
			model := config.ModelConfig{Provider: name, ModelID: "synthetic", WireFormat: "responses"}
			if _, err := p.Execute(ctx, commandCodeRequest(), model); err != nil {
				t.Fatal(err)
			}
			body, err := p.Stream(ctx, commandCodeRequest(), model)
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(io.Discard, body)
			_ = body.Close()
			if err != nil || calls != 2 {
				t.Fatalf("checked source not used: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestResponsesSnapshotSourceHealth(t *testing.T) {
	for _, mode := range []string{"consistent", "omitted-identity", "extended", "terminal-only", "changed-name", "changed-call", "changed-arguments", "invalid-delta", "budget-delta"} {
		t.Run(mode, func(t *testing.T) {
			item := types.ResponsesOutput{Type: "function_call", ID: "f", CallID: "call", Name: "lookup", Arguments: `{}`}
			if mode == "extended" || mode == "invalid-delta" || mode == "budget-delta" {
				item.Arguments = ""
			}
			var frames []string
			add := func(chunk types.ResponsesChunk) {
				data, err := json.Marshal(chunk)
				if err != nil {
					t.Fatal(err)
				}
				frames = append(frames, "data: "+string(data)+"\n\n")
			}
			if mode != "terminal-only" {
				add(types.ResponsesChunk{Type: "response.output_item.added", Item: &item})
			}
			response := &types.ResponsesResponse{Status: "completed", Usage: &types.ResponsesUsage{OutputTokens: 7, OutputTokensDetails: &types.CompletionTokensDetails{ReasoningTokens: 3}}}
			switch mode {
			case "omitted-identity":
				item.CallID, item.Name = "", ""
			case "changed-name":
				item.Name = "other"
			case "changed-call":
				item.CallID = "other"
			case "changed-arguments":
				item.Arguments = `[]`
			case "extended", "invalid-delta", "budget-delta":
				add(types.ResponsesChunk{Type: "response.function_call_arguments.delta", ItemID: "f", Delta: `{"x":`})
				if mode == "extended" {
					item.Arguments = `{"x":1}`
				}
			}
			if mode != "invalid-delta" && mode != "budget-delta" {
				response.Output = []types.ResponsesOutput{item}
			}
			kind := "response.completed"
			if mode == "budget-delta" {
				kind, response.Status = "response.incomplete", "incomplete"
				response.IncompleteDetails = &types.ResponsesIncompleteDetails{Reason: "max_output_tokens"}
			}
			add(types.ResponsesChunk{Type: kind, Response: response})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, strings.Join(frames, ""))
			}))
			defer upstream.Close()
			p, pool := selectedTestProvider("opencode-zen", upstream.URL)
			pool.stateFor("synthetic-a").consecutive = 1
			body, err := p.Stream(t.Context(), clinePassRequest(), config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "responses"})
			if err != nil {
				t.Fatal(err)
			}
			out := httptest.NewRecorder()
			err = transformer.NewStreamHandler().ProxyResponsesStream(out, body, "synthetic", t.Context(), 0, func() {})
			st := pool.states["synthetic-a"]
			valid := mode == "consistent" || mode == "omitted-identity" || mode == "extended" || mode == "terminal-only" || mode == "budget-delta"
			if valid {
				if err != nil || st.consecutive != 0 || st.inFlight != 0 || !strings.Contains(out.Body.String(), `"type":"message_stop"`) {
					t.Fatalf("valid source rejected: err=%v health=%+v output=%s", err, st, out.Body.String())
				}
				return
			}
			if err == nil || strings.Contains(out.Body.String(), `"type":"message_stop"`) {
				t.Fatalf("conflicting source accepted: err=%v output=%s", err, out.Body.String())
			}
			if st.inFlight != 0 || st.consecutive != 2 {
				t.Fatalf("source conflict was credited healthy: %+v", st)
			}
		})
	}
}

func TestResponsesNativeCumulativeArgumentsKeepSourceHealth(t *testing.T) {
	for _, mode := range []string{"native", "converted", "invalid-json"} {
		t.Run(mode, func(t *testing.T) {
			arguments := `{"x":"` + strings.Repeat("x", 2<<20) + `"}`
			if mode == "invalid-json" {
				arguments = arguments[:len(arguments)-1]
			}
			var wire strings.Builder
			add := func(chunk types.ResponsesChunk) {
				data, err := json.Marshal(chunk)
				if err != nil {
					t.Fatal(err)
				}
				if len(data) >= 1<<20 {
					t.Fatal("fixture exceeds the independent SSE frame limit")
				}
				wire.WriteString("data: " + string(data) + "\n\n")
			}
			add(types.ResponsesChunk{Type: "response.output_item.added", Item: &types.ResponsesOutput{Type: "function_call", ID: "f", CallID: "call", Name: "lookup"}})
			for i := 0; i < len(arguments); i += 1 << 18 {
				add(types.ResponsesChunk{Type: "response.function_call_arguments.delta", ItemID: "f", Delta: arguments[i:min(i+1<<18, len(arguments))]})
			}
			add(types.ResponsesChunk{Type: "response.completed", Response: &types.ResponsesResponse{Status: "completed"}})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, wire.String())
			}))
			defer upstream.Close()
			p, pool := selectedTestProvider("opencode-zen", upstream.URL)
			pool.stateFor("synthetic-a").consecutive = 1
			body, err := p.Stream(t.Context(), clinePassRequest(), config.ModelConfig{Provider: p.Name(), ModelID: "synthetic", WireFormat: "responses"})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "converted" {
				out := httptest.NewRecorder()
				err = transformer.NewStreamHandler().ProxyResponsesStream(out, body, "synthetic", t.Context(), 0, func() {})
				if err == nil || !strings.Contains(err.Error(), "exceed 1 MiB") || strings.Contains(out.Body.String(), `"type":"message_stop"`) {
					t.Fatalf("conversion cap lost: %v", err)
				}
				if st := pool.states["synthetic-a"]; st.consecutive != 1 || st.inFlight != 0 {
					t.Fatalf("conversion-only cap penalized source: %+v", st)
				}
				return
			}
			got, err := io.ReadAll(body)
			_ = body.Close()
			if err != nil || string(got) != wire.String() {
				t.Fatalf("native bytes changed: %v", err)
			}
			want := 0
			if mode == "invalid-json" {
				want = 2
			}
			if st := pool.states["synthetic-a"]; st.consecutive != want || st.inFlight != 0 {
				t.Fatalf("native arguments incorrectly classified: %+v, want failures=%d", st, want)
			}
		})
	}
}
