package transformer

import (
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestPublishedEffortDefaultHonorsExplicitConfig(t *testing.T) {
	for _, tc := range []struct {
		override string
		levels   []string
		want     string
	}{
		{"", nil, "max"}, {"low", nil, "low"}, {"", []string{"high"}, "high"},
	} {
		model := config.ModelConfig{Provider: "workbuddy", ModelID: "global:deepseek-v4.1-flash", ReasoningEffort: tc.override, PublishedReasoning: &config.PublishedReasoning{Levels: []string{"low", "high", "max"}, DefaultEffort: "max"}}
		if tc.levels != nil {
			model.Reasoning = &config.ReasoningConfig{Levels: tc.levels}
		}
		var request types.ChatCompletionRequest
		setReasoningEffort(&request, model)
		if request.ReasoningEffort == nil || *request.ReasoningEffort != tc.want {
			t.Fatalf("effort=%v want %s", request.ReasoningEffort, tc.want)
		}
	}
}
