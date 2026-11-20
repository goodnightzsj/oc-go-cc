package transformer

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// A block the Chat wire format has no field for used to be dropped and the
// request answered anyway: the model replied about a document it never
// received, and the client was told 200.
func TestUnrepresentableContentBlocksAreRefused(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"document", `[{"type":"text","text":"summarize"},{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}}]`},
		{"server_tool_use", `[{"type":"text","text":"look"},{"type":"server_tool_use","id":"s","name":"web_search","input":{}}]`},
		{"web_search_tool_result", `[{"type":"text","text":"look"},{"type":"web_search_tool_result","tool_use_id":"s","content":[]}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, role := range []string{"user", "assistant"} {
				req := &types.MessageRequest{
					MaxTokens: 16,
					Messages:  []types.Message{{Role: role, Content: json.RawMessage(tc.content)}},
				}
				_, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
				var conversion *core.RequestConversionError
				if !errors.As(err, &conversion) || !strings.Contains(err.Error(), tc.name) {
					t.Fatalf("%s/%s: err = %v, want a conversion error naming the block", role, tc.name, err)
				}
			}
		})
	}
	// Redacted thinking is opaque by construction and stays dropped: it is the
	// original provider's ciphertext and carries nothing a Chat upstream can use.
	req := &types.MessageRequest{MaxTokens: 16, Messages: []types.Message{{
		Role:    "assistant",
		Content: json.RawMessage(`[{"type":"redacted_thinking","data":"opaque"},{"type":"text","text":"hi"}]`),
	}}}
	if _, err := NewRequestTransformer().TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"}); err != nil {
		t.Fatalf("redacted_thinking: %v", err)
	}
}
