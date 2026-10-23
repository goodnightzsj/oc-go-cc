package provider

import "testing"

// Both spellings are what an operator actually types: the gateway's own panel
// shows a bare host, the OpenAI convention ends in /v1. Getting this wrong
// produces a 404 on every request, which reads like the gateway is down.
func TestChatCompletionsEndpoint(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"http://127.0.0.1:7863", "http://127.0.0.1:7863/v1/chat/completions"},
		{"http://127.0.0.1:7863/", "http://127.0.0.1:7863/v1/chat/completions"},
		{"http://127.0.0.1:7863/v1", "http://127.0.0.1:7863/v1/chat/completions"},
		{"http://127.0.0.1:7863/v1/", "http://127.0.0.1:7863/v1/chat/completions"},
		// Already complete: used as-is rather than doubled into
		// /v1/chat/completions/v1/chat/completions.
		{"http://127.0.0.1:7863/v1/chat/completions", "http://127.0.0.1:7863/v1/chat/completions"},
		{"  http://127.0.0.1:7863/v1  ", "http://127.0.0.1:7863/v1/chat/completions"},
		// Empty stays empty so the caller reports "no endpoint" rather than
		// posting to a relative URL.
		{"", ""},
		{"   ", ""},
	} {
		if got := chatCompletionsEndpoint(tc.in); got != tc.want {
			t.Errorf("chatCompletionsEndpoint(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
