package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/pkg/types"
)

func TestChatRefusalReachesBothInboundsAndHistory(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, path := range []string{"/v1/messages", "/v1/responses"} {
			for _, empty := range []bool{false, true} {
				for _, usage := range []string{"null", `{"prompt_tokens":0,"completion_tokens":0}`, `{"prompt_tokens":5,"completion_tokens":2}`} {
					t.Run(fmt.Sprintf("stream=%t%s/empty=%t/%s", stream, path, empty, usage), func(t *testing.T) {
						text, finish := "declined", "stop"
						if empty {
							text, finish = "", "content_filter"
						}
						var calls atomic.Int32
						srv := nativeResponsesServer(t, "commandcode", func(w http.ResponseWriter, r *http.Request) {
							calls.Add(1)
							_, _ = io.Copy(io.Discard, r.Body)
							if stream {
								w.Header().Set("Content-Type", "text/event-stream")
								_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"refusal\":%q},\"finish_reason\":%q}],\"usage\":%s}\n\ndata: [DONE]\n\n", text, finish, usage)
							} else {
								_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":null,"refusal":%q},"finish_reason":%q}],"usage":%s}`, text, finish, usage)
							}
						}, func(cfg *config.Config) {
							model := cfg.Models["default"]
							model.WireFormat = "openai"
							cfg.Models["default"] = model
						})
						payload := fmt.Sprintf(`{"model":"m","max_tokens":100,"messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)
						if path == "/v1/responses" {
							payload = fmt.Sprintf(`{"model":"m","input":"hi","stream":%t}`, stream)
						}
						w := httptest.NewRecorder()
						srv.httpSrv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload)))
						marker := `"refusal"`
						if path == "/v1/responses" {
							marker = `"content_filter"`
						}
						if w.Code != 200 || calls.Load() != 1 || !strings.Contains(w.Body.String(), marker) {
							t.Fatalf("lost refusal: calls=%d HTTP %d %s", calls.Load(), w.Code, w.Body.String())
						}
						if got := chatRefusalVisibleText(t, w.Body.String(), stream, path == "/v1/responses"); got != text {
							t.Fatalf("text=%q want=%q", got, text)
						}
						rows, total, err := storage.NewRequests(srv.Storage()).Query(t.Context(), storage.RequestQuery{})
						if err != nil || total != 1 || !rows[0].Success || !rows[0].OutcomeKnown {
							t.Fatalf("history=%+v total=%d err=%v", rows, total, err)
						}
						row := rows[0]
						wantTokens := 0
						if strings.Contains(usage, "5") {
							wantTokens = 2
						}
						if row.OutputTokens != wantTokens || row.DetailsKnown != (usage != "null") {
							t.Fatalf("usage changed: %+v", row)
						}
						if usage == "null" && row.CostKnown {
							t.Fatalf("missing usage priced: %+v", row)
						}
					})
				}
			}
		}
	}
}

func chatRefusalVisibleText(t *testing.T, raw string, stream, responses bool) string {
	t.Helper()
	var text string
	if stream {
		for _, line := range strings.Split(raw, "\n") {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event struct {
				Type  string
				Delta json.RawMessage
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "response.output_text.delta" {
				var delta string
				if err := json.Unmarshal(event.Delta, &delta); err != nil {
					t.Fatal(err)
				}
				text += delta
			} else if event.Type == "content_block_delta" {
				var delta types.Delta
				if err := json.Unmarshal(event.Delta, &delta); err != nil {
					t.Fatal(err)
				}
				text += delta.Text
			}
		}
	} else if responses {
		var response types.ResponsesResponse
		if err := json.Unmarshal([]byte(raw), &response); err != nil {
			t.Fatal(err)
		}
		for _, item := range response.Output {
			for _, part := range item.Content {
				text += part.Text
			}
		}
	} else {
		var response types.MessageResponse
		if err := json.Unmarshal([]byte(raw), &response); err != nil {
			t.Fatal(err)
		}
		for _, block := range response.Content {
			text += block.Text
		}
	}
	return text
}
