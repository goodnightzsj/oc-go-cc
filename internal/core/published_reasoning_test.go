package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/site"
)

func TestPublishedReasoningPrecedence(t *testing.T) {
	model := config.ModelConfig{
		Provider: site.WorkBuddy, ModelID: "global:deepseek-v4.1-flash",
		PublishedReasoning: &config.PublishedReasoning{Levels: []string{"low", "high", "max"}, DefaultEffort: "max", CanDisableThinking: config.BoolPtr(true)},
	}
	got := ReasoningSupportFor(model)
	if !reflect.DeepEqual(got.Levels, model.PublishedReasoning.Levels) || got.DefaultEffort != "max" || !got.EffortDriven || !got.Budget || !got.TakesThinking || got.CannotDisable {
		t.Fatalf("published ladder must override only its declared fields: %+v", got)
	}
	model.Reasoning = &config.ReasoningConfig{Levels: []string{"medium"}, Budget: config.BoolPtr(false), CannotDisable: config.BoolPtr(true)}
	got = ReasoningSupportFor(model)
	if effort, ok := got.ClampLevel("max"); !ok || effort != "medium" || got.Budget || !got.CannotDisable {
		t.Fatalf("operator override lost precedence: %+v", got)
	}
	model.Reasoning = nil
	model.PublishedReasoning = &config.PublishedReasoning{Levels: []string{}}
	if got, want := ReasoningSupportFor(model), ReasoningSupportFor(config.ModelConfig{Provider: model.Provider, ModelID: model.ModelID}); !reflect.DeepEqual(got, want) {
		t.Fatalf("empty/unknown directory must retain builtin: %+v, want %+v", got, want)
	}
}

func TestWorkBuddyKnownFallbackLaddersArePlatformSpecific(t *testing.T) {
	for _, tc := range []struct{ id, intent, want string }{
		{"global:deepseek-v4.1-flash", "xhigh", "high"},
		{"global:deepseek-v4.1-flash", "max", "max"},
		{"global:kimi-k3", "max", "medium"},
	} {
		got, ok := ReasoningSupportFor(config.ModelConfig{Provider: site.WorkBuddy, ModelID: tc.id}).ClampLevel(tc.intent)
		if !ok || got != tc.want {
			t.Fatalf("%s %s became %q; want %s", tc.id, tc.intent, got, tc.want)
		}
		other, ok := ReasoningSupportFor(config.ModelConfig{Provider: site.OpenRouter, ModelID: tc.id}).ClampLevel(tc.intent)
		if !ok || other != tc.intent {
			t.Fatalf("WorkBuddy fallback changed OpenRouter: %s %q", tc.id, other)
		}
	}
}

func TestPublishedReasoningIsPlatformScopedAndNotPersisted(t *testing.T) {
	for _, provider := range []string{site.ClinePass, site.OpenRouter, site.OpenCodeGo} {
		model := config.ModelConfig{Provider: provider, ModelID: "global:kimi-k3"}
		want := ReasoningSupportFor(model)
		model.PublishedReasoning = &config.PublishedReasoning{Levels: []string{"low"}, DefaultEffort: "low", CanDisableThinking: config.BoolPtr(true)}
		if got := ReasoningSupportFor(model); !reflect.DeepEqual(got, want) {
			t.Errorf("%s used WorkBuddy metadata: %+v", provider, got)
		}
		body, err := json.Marshal(model)
		if err != nil || strings.Contains(string(body), "PublishedReasoning") || strings.Contains(string(body), "DefaultEffort") {
			t.Fatalf("runtime roster metadata persisted: %s, %v", body, err)
		}
	}
}

func TestPublishedThinkingSwitchOverridesBuiltin(t *testing.T) {
	for _, canDisable := range []bool{false, true} {
		model := config.ModelConfig{Provider: site.WorkBuddy, ModelID: "global:kimi-k3", PublishedReasoning: &config.PublishedReasoning{CanDisableThinking: config.BoolPtr(canDisable)}}
		got := ReasoningSupportFor(model)
		if got.CannotDisable == canDisable || (canDisable && (!got.TakesThinking || got.NoOffLevel)) {
			t.Fatalf("can_disable_thinking=%t ignored: %+v", canDisable, got)
		}
	}
}

func TestPublishedOffIsNeverANumericRung(t *testing.T) {
	model := config.ModelConfig{Provider: site.WorkBuddy, ModelID: "global:kimi-k3", PublishedReasoning: &config.PublishedReasoning{Levels: []string{"low", "high", "off"}, CanDisableThinking: config.BoolPtr(true)}}
	got := ReasoningSupportFor(model)
	if !reflect.DeepEqual(got.Levels, []string{"low", "high"}) {
		t.Fatalf("off polluted numeric ladder: %+v", got)
	}
	if level, ok := got.ClampLevel("max"); !ok || level != "high" {
		t.Fatalf("max became %q, %t instead of high", level, ok)
	}
	if _, ok := got.ClampLevel("none"); ok || !got.TakesThinking || got.CannotDisable {
		t.Fatalf("off did not remain an independent switch: %+v", got)
	}
}
