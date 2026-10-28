package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/site"
)

func TestPublishedByActiveSiteUsesWorkBuddyEffortDirectory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"data":[{"id":"global:kimi-k3","reasoning_supported_efforts":["medium"]}]}`)
	}))
	defer srv.Close()
	r := NewModelRouter(newTestAtomicConfig(&config.Config{
		ActiveSite: site.WorkBuddy,
		WorkBuddy:  config.WorkBuddyConfig{BaseURL: srv.URL, APIKey: "synthetic-key"},
	}))
	model, ok := r.PublishedByActiveSite(context.Background(), "global:kimi-k3")
	if !ok {
		t.Fatal("published model was not resolved")
	}
	if effort, ok := core.ReasoningSupportFor(model).ClampLevel("max"); !ok || effort != "medium" {
		t.Fatalf("published Kimi ladder lost: max became %q, %t; want medium", effort, ok)
	}
}

func TestPublishedReasoningEnrichesWholeChainWithoutChangingIdentity(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprint(w, `{"data":[
			{"id":"global:kimi-k3","reasoning_supported_efforts":["medium"]},
			{"id":"cn:kimi-k3","reasoning_supported_efforts":["low","high","max"]},
			{"id":"global:deepseek-v4.1-flash","reasoning_supported_efforts":["low","high","max","off"]}
		]}`)
	}))
	defer srv.Close()
	cfg := &config.Config{
		ActiveSite:           site.ClinePass,
		WorkBuddy:            config.WorkBuddyConfig{BaseURL: srv.URL, APIKey: "synthetic-key"},
		Models:               map[string]config.ModelConfig{"default": {Provider: site.WorkBuddy, ModelID: "global:kimi-k3"}},
		ModelOverrides:       map[string]config.ModelConfig{"exact": {Provider: site.WorkBuddy, ModelID: "global:kimi-k3"}},
		ModelFamilyOverrides: map[string]config.ModelConfig{"opus": {Provider: site.WorkBuddy, ModelID: "global:kimi-k3"}},
		Fallbacks: map[string][]config.ModelConfig{"default": {
			{Provider: site.WorkBuddy, ModelID: "cn:kimi-k3"},
			{Provider: site.WorkBuddy, ModelID: "global:kimi-k3", Reasoning: &config.ReasoningConfig{Levels: []string{"high"}}},
			{Provider: site.OpenRouter, ModelID: "global:kimi-k3"},
			{Provider: site.WorkBuddy, ModelID: "global:kimi-k3.1-pro"},
		}},
	}
	r := NewModelRouter(newTestAtomicConfig(cfg))
	for _, route := range []string{"scenario", "stream", "requested", "override", "family"} {
		t.Run(route, func(t *testing.T) {
			var result RouteResult
			var err error
			switch route {
			case "scenario":
				result, err = r.Route(nil, 1, "")
			case "stream":
				result, err = r.RouteForStreaming(nil, 1, "")
				// Streaming fallback selection is independently configured.
				result.Fallbacks = cfg.Fallbacks["default"]
			case "requested":
				result, err = r.Route(nil, 1, "default")
			case "override":
				result, _ = r.RouteWithOverride("exact")
			case "family":
				result, _ = r.RouteWithFamilyOverride("claude-opus-test")
			}
			if err != nil {
				t.Fatal(err)
			}
			chain := result.GetModelChain()
			got := r.ApplyPublishedReasoning(context.Background(), chain)
			wantEfforts := []string{"medium", "max", "high", "max", "max"}
			if len(got) != len(wantEfforts) {
				t.Fatalf("chain = %+v", got)
			}
			for i, model := range got {
				if config.ModelKey(model) != config.ModelKey(chain[i]) || model.Reasoning != chain[i].Reasoning {
					t.Fatalf("target/config changed at %d", i)
				}
				if effort, ok := core.ReasoningSupportFor(model).ClampLevel("max"); !ok || effort != wantEfforts[i] {
					t.Errorf("target %d got %q,%t, want %q", i, effort, ok, wantEfforts[i])
				}
				if chain[i].PublishedReasoning != nil {
					t.Fatal("input chain was mutated")
				}
			}
			// Runtime values are independent of both the cache and the next request.
			got[0].PublishedReasoning.Levels[0] = "low"
			again := r.ApplyPublishedReasoning(context.Background(), chain)
			if !reflect.DeepEqual(again[0].PublishedReasoning.Levels, []string{"medium"}) {
				t.Fatal("returned metadata aliases the cache")
			}
		})
	}
	if calls.Load() != 1 {
		t.Fatalf("chain enrichment bypassed roster cache: %d calls", calls.Load())
	}
}

func TestPublishedReasoningFailureDoesNotReuseOldGatewayCapabilities(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if req.Header.Get("Authorization") == "Bearer synthetic-new" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"global:kimi-k3","reasoning_supported_efforts":["medium"]}]}`)
	}))
	defer srv.Close()
	cfg := &config.Config{WorkBuddy: config.WorkBuddyConfig{BaseURL: srv.URL, APIKey: "synthetic-old"}}
	a := newTestAtomicConfig(cfg)
	r := NewModelRouter(a)
	chain := []config.ModelConfig{{Provider: site.WorkBuddy, ModelID: "global:kimi-k3"}}
	old := r.ApplyPublishedReasoning(context.Background(), chain)
	if old[0].PublishedReasoning == nil {
		t.Fatal("initial metadata missing")
	}
	next := *cfg
	next.WorkBuddy.APIKey = "synthetic-new"
	a.ApplyLoaded(&next)
	for range 2 {
		got := r.ApplyPublishedReasoning(context.Background(), old)
		if got[0].PublishedReasoning != nil {
			t.Fatal("old credential's capability survived a failed new discovery")
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("failed discovery was not briefly cached: %d calls", calls.Load())
	}
	r.siteMu.Lock()
	entry := r.siteCache[site.WorkBuddy]
	entry.expires = time.Now().Add(-time.Second)
	r.siteCache[site.WorkBuddy] = entry
	r.siteMu.Unlock()
	r.ApplyPublishedReasoning(context.Background(), chain)
	if calls.Load() != 3 {
		t.Fatal("expired failure did not retry")
	}
}

func TestPublishedReasoningConcurrentDiscoveryAndCancellation(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprint(w, `{"data":[{"id":"global:kimi-k3","reasoning_supported_efforts":["medium"]}]}`)
	}))
	defer srv.Close()
	r := NewModelRouter(newTestAtomicConfig(&config.Config{WorkBuddy: config.WorkBuddyConfig{BaseURL: srv.URL, APIKey: "synthetic-key"}}))
	chain := []config.ModelConfig{{Provider: site.WorkBuddy, ModelID: "global:kimi-k3"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.ApplyPublishedReasoning(ctx, chain)
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			got := r.ApplyPublishedReasoning(context.Background(), chain)
			if got[0].PublishedReasoning == nil {
				t.Error("cancelled discovery poisoned another caller")
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent discovery fetched %d times", calls.Load())
	}
}

func TestSiteRosterCacheTracksEndpointAndCredential(t *testing.T) {
	var firstCalls, secondCalls atomic.Int32
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		firstCalls.Add(1)
		id := "global:first"
		if req.Header.Get("Authorization") == "Bearer synthetic-key-b" {
			id = "global:second-key"
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"id":%q}]}`, id)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondCalls.Add(1)
		_, _ = fmt.Fprint(w, `{"data":[{"id":"global:second-host"}]}`)
	}))
	defer second.Close()
	cfg := &config.Config{ActiveSite: site.WorkBuddy, WorkBuddy: config.WorkBuddyConfig{BaseURL: first.URL, APIKey: "synthetic-key-a"}}
	a := newTestAtomicConfig(cfg)
	r := NewModelRouter(a)
	assertID := func(want string) {
		t.Helper()
		models := r.ListModels(context.Background())
		if len(models) != 1 || models[0].ID != want {
			t.Fatalf("roster = %+v, want only %s", models, want)
		}
	}
	assertID("global:first")
	assertID("global:first")
	if firstCalls.Load() != 1 {
		t.Fatalf("unchanged scope fetched %d times", firstCalls.Load())
	}
	next := *cfg
	next.WorkBuddy.APIKey = "synthetic-key-b"
	a.ApplyLoaded(&next)
	assertID("global:second-key")
	nextHost := next
	nextHost.WorkBuddy.BaseURL = second.URL
	a.ApplyLoaded(&nextHost)
	assertID("global:second-host")
	if firstCalls.Load() != 2 || secondCalls.Load() != 1 {
		t.Fatalf("fetch counts = %d / %d, want 2 / 1", firstCalls.Load(), secondCalls.Load())
	}
}
