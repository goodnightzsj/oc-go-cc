package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestClinePassRefusalProjection(t *testing.T) {
	for _, tc := range []struct {
		name, chunks, text string
		refusal            bool
	}{
		{"interleaved", `{"refusal":"R1"}` + "\n" + `{"content":"C"}` + "\n" + `{"refusal":"R2"}`, "R1CR2", true},
		{"same-fields", `{"content":"X","refusal":"X"}`, "XX", true},
		{"empty", `{"refusal":""}`, "", false},
		{"null", `{"refusal":null}`, "", false},
	} {
		for _, finish := range []string{"stop", "length", "content_filter", "tool_calls", "unknown", ""} {
			t.Run(tc.name+"/"+finish, func(t *testing.T) {
				var wire strings.Builder
				for _, delta := range strings.Split(tc.chunks, "\n") {
					fmt.Fprintf(&wire, "data: {\"choices\":[{\"delta\":%s}]}\n\n", delta)
				}
				fmt.Fprintf(&wire, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":%q}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n", finish)
				p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, wire.String()) })
				result, err := p.Execute(t.Context(), clinePassRequest(), config.ModelConfig{Provider: "cline-pass", ModelID: "synthetic"})
				wantStop := map[string]string{"stop": "end_turn", "length": "max_tokens", "content_filter": "refusal", "tool_calls": "tool_use"}[finish]
				if finish == "stop" && tc.refusal {
					wantStop = "refusal"
				}
				if wantStop == "" {
					if err == nil {
						t.Fatal("invalid terminal accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var response types.MessageResponse
				if err := json.Unmarshal(result.Body, &response); err != nil {
					t.Fatal(err)
				}
				var text string
				for _, block := range response.Content {
					if block.Type == "text" {
						text += block.Text
					}
				}
				if text != tc.text || response.StopReason != wantStop || response.Usage == nil || response.Usage.InputTokens != 5 || response.Usage.OutputTokens != 2 {
					t.Fatalf("projection lost: %+v text=%q want=%q", response, text, tc.text)
				}
			})
		}
	}
}

func TestClinePassRefusalStillRequiresCompleteSource(t *testing.T) {
	const refusal = "data: {\"choices\":[{\"delta\":{\"refusal\":\"declined\"}}]}\n\n"
	const finish = "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
	for _, wire := range []string{refusal, refusal + finish, refusal + finish + refusal + "data: [DONE]\n\n", refusal + "data: {\"error\":{\"message\":\"failed\"}}\n\ndata: [DONE]\n\n"} {
		if _, err := aggregateChatStream(strings.NewReader(wire), "synthetic"); err == nil {
			t.Fatalf("invalid refusal stream accepted: %s", wire)
		}
	}
}
