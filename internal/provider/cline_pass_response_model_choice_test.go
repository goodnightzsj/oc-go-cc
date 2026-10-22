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
	"github.com/routatic/proxy/pkg/types"
)

// A non-streaming answer that names its model in the chunks must report that
// name, not the id the client asked for. Before this, only the trailing
// routing block could supply it; a stream without one reported the request id
// even though the chunks had said otherwise.
func TestExecuteNamesTheModelFromTheStreamWhenRoutingBlockIsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		chunkModel string
		routing    string
		want       string
	}{
		{"chunks name it, no routing block", "deepseek/deepseek-v4.1-flash-1017", "", "deepseek/deepseek-v4.1-flash-1017"},
		{"routing block wins over chunks", "deepseek/from-chunk", "vmc/real-one", "vmc/real-one"},
		{"neither names it", "", "", "cline-pass/deepseek-v4.1-flash-1017"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				chunk := map[string]any{"id": "c1", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "hi"}}}}
				if tc.chunkModel != "" {
					chunk["model"] = tc.chunkModel
				}
				b, _ := json.Marshal(chunk)
				_, _ = io.WriteString(w, "data: "+string(b)+"\n\n")
				if tc.routing != "" {
					meta := map[string]any{"choices": []any{}, "provider_metadata": map[string]any{"gateway": map[string]any{"routing": map[string]any{"canonicalSlug": tc.routing}}}}
					mb, _ := json.Marshal(meta)
					_, _ = io.WriteString(w, "data: "+string(mb)+"\n\n")
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer upstream.Close()

			cfg := &config.Config{ClinePass: config.ClinePassConfig{BaseURL: upstream.URL + "/api/v1/chat/completions", APIKeys: []string{"synthetic"}}}
			p := NewClinePassProvider(config.NewAtomicConfig(cfg, ""), nil)
			model := config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/deepseek-v4.1-flash-1017"}
			req := &types.MessageRequest{Model: model.ModelID, MaxTokens: 8, Messages: []types.Message{{Role: "user", Content: json.RawMessage(`"hi"`)}}}

			result, err := p.Execute(context.Background(), req, model)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(result.Body, &got); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got.Model, tc.want) {
				t.Errorf("response model = %q, want %q", got.Model, tc.want)
			}
		})
	}
}
