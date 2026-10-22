package core

import (
	"context"
	"io"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

// stubProvider reports one wire format for every model.
type stubProvider struct{ format WireFormat }

func (s stubProvider) Name() string                 { return "stub" }
func (s stubProvider) WireFormat(string) WireFormat { return s.format }
func (s stubProvider) Execute(context.Context, *types.MessageRequest, config.ModelConfig) (*ExecuteResult, error) {
	return nil, nil
}
func (s stubProvider) Stream(context.Context, *types.MessageRequest, config.ModelConfig) (io.ReadCloser, error) {
	return nil, nil
}

// anthropic_tools_disabled exists so a model that cannot take raw Anthropic
// tool definitions is sent through the transform instead. It was read from
// config and copied into every target, but nothing consulted it: setting it
// changed nothing at all.
func TestAnthropicToolsDisabledDowngradesAnInferredAnthropicEndpoint(t *testing.T) {
	anthropic := stubProvider{format: WireFormatAnthropic}

	if got := ModelWireFormat(anthropic, config.ModelConfig{ModelID: "claude-x"}); got != WireFormatAnthropic {
		t.Fatalf("without the flag the endpoint must stay Anthropic, got %v", got)
	}
	got := ModelWireFormat(anthropic, config.ModelConfig{ModelID: "claude-x", AnthropicToolsDisabled: true})
	if got != WireFormatOpenAIChat {
		t.Errorf("with the flag set the endpoint must downgrade to Chat Completions, got %v", got)
	}

	// An explicit wire_format is the operator's own statement and is not
	// overridden - a visibly failing request beats a silently different one.
	got = ModelWireFormat(anthropic, config.ModelConfig{ModelID: "claude-x", WireFormat: "anthropic", AnthropicToolsDisabled: true})
	if got != WireFormatAnthropic {
		t.Errorf("explicit wire_format must win, got %v", got)
	}

	// Providers that already speak Chat Completions are unaffected.
	openai := stubProvider{format: WireFormatOpenAIChat}
	if got := ModelWireFormat(openai, config.ModelConfig{ModelID: "m", AnthropicToolsDisabled: true}); got != WireFormatOpenAIChat {
		t.Errorf("a Chat Completions provider must be unchanged, got %v", got)
	}
}
