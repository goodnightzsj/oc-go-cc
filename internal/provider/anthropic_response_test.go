package provider

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func TestNativeAnthropicResponseBoundary(t *testing.T) {
	for _, name := range []string{"opencode-go", "opencode-zen", "aws-bedrock", "commandcode"} {
		for _, tc := range []struct {
			name, body string
			valid      bool
		}{
			{"error", `{"type":"error","error":{"type":"overloaded_error","message":"synthetic"},"usage":{"input_tokens":2}}`, false},
			{"malformed", `{"type":`, false},
			{"unfinished", `{"type":"message","content":[],"stop_reason":null}`, false},
			{"native", "{\n\"type\":\"message\",\"model\":\"reported\",\"content\":[{\"type\":\"thinking\",\"thinking\":\"thought\",\"signature\":\"opaque\"}],\"stop_reason\":\"end_turn\",\"future_field\":true}", true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, tc.body) }))
				defer upstream.Close()
				p, pool := selectedTestProvider(name, upstream.URL)
				_, _, req := cancellationTestProvider("")
				req.MaxTokens = 16
				pool.stateFor("synthetic-a").consecutive = 1
				result, err := p.Execute(t.Context(), req, config.ModelConfig{Provider: name, ModelID: "synthetic", WireFormat: "anthropic"})
				if (err == nil) != tc.valid {
					t.Fatalf("valid=%t result=%v err=%v", tc.valid, result, err)
				}
				wantFailures := 2
				if tc.valid {
					wantFailures = 0
					if string(result.Body) != tc.body || result.ResponseModel != "reported" {
						t.Fatalf("valid native bytes/model changed: %+v", result)
					}
				}
				if got := pool.states["synthetic-a"].consecutive; got != wantFailures {
					t.Fatalf("key failures=%d want=%d", got, wantFailures)
				}
			})
		}
	}
}
