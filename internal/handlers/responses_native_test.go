package handlers

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
)

func TestResponsesChainKeepsNativeOnlyAndReplayOnSource(t *testing.T) {
	h, _ := newResponsesTestHandler(t, http.NotFoundHandler())
	primary := config.ModelConfig{Provider: "commandcode", ModelID: "synthetic", WireFormat: "responses"}
	chat := config.ModelConfig{Provider: "commandcode", ModelID: "chat", WireFormat: "openai"}
	other := config.ModelConfig{Provider: "commandcode", ModelID: "other", WireFormat: "responses"}
	chain := []config.ModelConfig{primary, chat, other}
	for _, tc := range []struct {
		name      string
		requested string
		bridgeErr error
		replay    bool
		keys      []string
		want      []config.ModelConfig
	}{
		{"compatible", "alias", nil, false, nil, chain},
		{"native-only", "alias", errors.New("custom tool"), false, nil, []config.ModelConfig{primary, other}},
		{"replay", "synthetic", errors.New("encrypted item"), true, nil, []config.ModelConfig{primary}},
		{"replay alias", "alias", errors.New("encrypted item"), true, nil, nil},
		{"replay pool", "synthetic", errors.New("encrypted item"), true, []string{"synthetic-a", "synthetic-b"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := *h.modelRouter.Config()
			snapshot.CommandCode.APIKeys = tc.keys
			got, err := h.responsesChain(core.RequestMetadata{ResponsesBody: []byte(`{}`), ResponsesBridgeError: tc.bridgeErr, ResponsesReplay: tc.replay, ResponsesSource: &snapshot}, tc.requested, chain)
			if (err != nil) != (tc.want == nil) || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%+v err=%v want=%+v", got, err, tc.want)
			}
		})
	}
	got, err := h.responsesChain(core.RequestMetadata{ResponsesBody: []byte(`{}`), ResponsesReplay: true}, "alias", []config.ModelConfig{chat, primary, other})
	if err != nil || !reflect.DeepEqual(got, []config.ModelConfig{chat}) {
		t.Fatalf("summary bridge exposed source ID to native fallback: %+v err=%v", got, err)
	}
}
