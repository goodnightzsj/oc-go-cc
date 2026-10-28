package catalog

import (
	"reflect"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

func TestWorkBuddyReasoningDirectory(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		levels       []string
		def          string
		disable      *bool
		unknown      bool
		wantErr      bool
	}{
		{name: "missing", unknown: true},
		{name: "null", fields: `,"reasoning_supported_efforts":null`, unknown: true},
		{name: "empty is unknown", fields: `,"reasoning_supported_efforts":[]`, unknown: true},
		{name: "single rung", fields: `,"reasoning_supported_efforts":["medium"]`, levels: []string{"medium"}},
		{name: "default", fields: `,"reasoning_supported_efforts":["low","high","max"],"reasoning_default_effort":"max"`, levels: []string{"low", "high", "max"}, def: "max"},
		{name: "off is a switch", fields: `,"reasoning_supported_efforts":["low","high","off"],"can_disable_thinking":false`, levels: []string{"low", "high", "off"}, disable: config.BoolPtr(true)},
		{name: "cannot disable", fields: `,"can_disable_thinking":false`, disable: config.BoolPtr(false)},
		{name: "can disable", fields: `,"can_disable_thinking":true`, disable: config.BoolPtr(true)},
		{name: "canonicalize", fields: `,"reasoning_supported_efforts":[" LOW ","low","MAX"],"reasoning_default_effort":"max"`, levels: []string{"low", "max"}, def: "max"},
		{name: "invalid level", fields: `,"reasoning_supported_efforts":["future-level"]`, wantErr: true},
		{name: "blank level", fields: `,"reasoning_supported_efforts":[" "]`, wantErr: true},
		{name: "invalid array", fields: `,"reasoning_supported_efforts":"high"`, wantErr: true},
		{name: "invalid default type", fields: `,"reasoning_default_effort":42`, wantErr: true},
		{name: "default outside ladder", fields: `,"reasoning_supported_efforts":["high"],"reasoning_default_effort":"max"`, wantErr: true},
		{name: "invalid switch", fields: `,"can_disable_thinking":"false"`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models, err := parseSiteModels(site.WorkBuddy, []byte(`{"data":[{"id":"global:example"`+tc.fields+`}]}`))
			if tc.wantErr {
				if err == nil || models != nil {
					t.Fatalf("malformed metadata accepted: %+v, %v", models, err)
				}
				return
			}
			if err != nil || len(models) != 1 {
				t.Fatalf("models = %+v, %v", models, err)
			}
			got := models[0].Reasoning
			if tc.unknown {
				if got != nil {
					t.Fatalf("unknown metadata became a declaration: %+v", got)
				}
				return
			}
			if got == nil || !reflect.DeepEqual(got.Levels, tc.levels) || got.DefaultEffort != tc.def || !reflect.DeepEqual(got.CanDisableThinking, tc.disable) {
				t.Fatalf("reasoning = %+v; want levels=%v default=%q disable=%v", got, tc.levels, tc.def, tc.disable)
			}
		})
	}
}

func TestWorkBuddyReasoningDoesNotInterpretOtherPlatformsMetadata(t *testing.T) {
	models, err := parseSiteModels(site.CommandCode, []byte(`{"data":[{"id":"example","reasoning_supported_efforts":{"vendor":"shape"},"can_disable_thinking":"unknown"}]}`))
	if err != nil || len(models) != 1 || models[0].Reasoning != nil {
		t.Fatalf("WorkBuddy schema affected another platform: %+v, %v", models, err)
	}
}
