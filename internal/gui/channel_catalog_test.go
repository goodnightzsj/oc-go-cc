package gui

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/debug"
)

func appendChannelCapture(t *testing.T, path, phase, id, body string) {
	t.Helper()
	data, err := json.Marshal(debug.CaptureEntry{Phase: phase, Provider: "cline-pass", RequestID: id, Data: body})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func channelSSE(channel, model string) string {
	return `data: {"model":"` + model + `"}` + "\n\n" + `data: {"choices":[{"delta":{"content":"private-answer-sentinel","provider_metadata":{"gateway":{"routing":{"finalProvider":"` + channel + `","fallbacksAvailable":["novita","novita","private answer sentinel"]}}}}}]}` + "\n\ndata: [DONE]\n\n"
}

func TestChannelCatalogIncrementalAndPrivacy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture-001.jsonl")
	request := `{"model":"cline-pass/synthetic-model","provider":{"only":["codex-no-such-channel"]},"messages":[{"content":"private-prompt-sentinel"}],"api_key":"private-key-sentinel"}`
	appendChannelCapture(t, path, debug.PhaseUpstreamRequest, "one", request)
	appendChannelCapture(t, path, debug.PhaseUpstreamRequest, "one", `{"model":"different-model-same-request-id"}`)
	appendChannelCapture(t, path, debug.PhaseUpstreamResponse, "one", channelSSE("fireworks", "upstream/synthetic-model"))
	var catalog channelCatalog
	if err := catalog.refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "channel-catalog.json")
	data, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"private-", "codex-no-such-channel", "messages", "api_key", "request_id"} {
		if bytes.Contains(data, []byte(sentinel)) {
			t.Fatalf("catalogue persisted non-channel data: %s", sentinel)
		}
	}
	got := catalog.snapshot(dir).Channels["upstream/synthetic-model"]
	if !reflect.DeepEqual(got.Actual, []string{"fireworks"}) || !reflect.DeepEqual(got.Available, []string{"novita"}) {
		t.Fatalf("metadata = %+v", got)
	}
	stamp := time.Unix(100, 0)
	if err := os.Chtimes(cache, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	appendChannelCapture(t, path, debug.PhaseUpstreamRequest, "two", request)
	appendChannelCapture(t, path, debug.PhaseUpstreamResponse, "two", channelSSE("fireworks", "upstream/synthetic-model"))
	if err := catalog.refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cache)
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Fatalf("unchanged channels rewrote JSON: %v", err)
	}
	// A trailing write fragment is not a malformed record and is not consumed.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	fragment := `{"phase":"upstream-response"`
	if _, err := file.WriteString(fragment); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if err := catalog.refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(path)
	if catalog.positions[filepath.Base(path)].offset != info.Size()-int64(len(fragment)) {
		t.Fatal("partial record consumed")
	}
	// A different capture directory must never inherit this directory's channels.
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "capture-002.jsonl"), []byte("not JSON\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := catalog.refresh(context.Background(), other); err == nil {
		t.Fatal("bad capture must report failure")
	}
	if !reflect.DeepEqual(catalog.snapshot(other).Channels, defaultChannels) || catalog.snapshot(other).Error == "" {
		t.Fatal("bad new source leaked old channels or hid failure")
	}
}

func TestChannelCatalogDisabledAndHourly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.Config{Logging: config.LoggingConfig{DebugCapture: &config.DebugCapture{Enabled: true, Directory: dir}}}
		srv := &Server{atomicCfg: config.NewAtomicConfig(cfg, "unused")}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); srv.channelLoop(ctx) }()
		synctest.Wait()
		if srv.channels.snapshot(dir).LastChecked == nil {
			t.Fatal("startup scan missing")
		}
		path := filepath.Join(dir, "capture-001.jsonl")
		appendChannelCapture(t, path, debug.PhaseUpstreamRequest, "one", `{"model":"cline-pass/hourly-model"}`)
		appendChannelCapture(t, path, debug.PhaseUpstreamResponse, "one", channelSSE("zai", "upstream/hourly-model"))
		time.Sleep(time.Hour)
		synctest.Wait()
		if got := srv.channels.snapshot(dir).Channels["upstream/hourly-model"].Actual; !reflect.DeepEqual(got, []string{"z-ai"}) {
			t.Fatalf("hourly metadata not updated: %v", got)
		}
		disabled := &config.Config{Logging: config.LoggingConfig{DebugCapture: &config.DebugCapture{Directory: filepath.Join(dir, "absent")}}}
		srv.atomicCfg.ApplyLoaded(disabled)
		time.Sleep(time.Hour)
		synctest.Wait()
		rec := httptest.NewRecorder()
		srv.handleChannelCatalog(rec, httptest.NewRequest(http.MethodGet, "/api/cline-pass/channels", nil))
		var response channelSnapshot
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.CaptureEnabled || response.Error != "" || response.DefaultTarget != "deepseek" || !reflect.DeepEqual(response.Channels, defaultChannels) {
			t.Fatal("disabled capture must always use the built-in JSON")
		}
		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("scanner did not stop")
		}
	})
}

func TestChannelResponseEvidenceAndCacheValidation(t *testing.T) {
	found := make(channelData)
	if err := readChannelResponse(strings.ReplaceAll(channelSSE("fireworks", "upstream/test"), "data: [DONE]", ""), found); err != nil || len(found) != 0 {
		t.Fatal("incomplete SSE became channel evidence")
	}
	if err := readChannelResponse(`{"model":"cline-pass/test","provider_metadata":{"gateway":{"routing":{"finalProvider":"deepseek"}}}}`, found); err != nil {
		t.Fatal(err)
	}
	if got := mergeChannels(nil, found)["cline-pass/test"].Actual; !reflect.DeepEqual(got, []string{"deepseek"}) {
		t.Fatalf("JSON metadata missing: %v", got)
	}
	for _, raw := range []string{`null`, `{}`, `{"test":{"actual":["deepseek"],"messages":"private"}}`, `{} {}`} {
		_, err := decodeChannelData([]byte(raw))
		if (err == nil) != (raw == `{}`) {
			t.Fatalf("cache validity %q: %v", raw, err)
		}
	}
}

func TestChannelCatalogRotationAndWriteRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture-001.jsonl")
	// Orphaned requests and reused client IDs cannot consume correlation state:
	// the catalogue trusts only response-owned metadata.
	orphans := strings.Repeat(`{"phase":"upstream-request","provider":"cline-pass","request_id":"same","data":"private-prompt-sentinel"}`+"\n", 10001)
	if err := os.WriteFile(path, []byte(orphans), 0600); err != nil {
		t.Fatal(err)
	}
	var catalog channelCatalog
	if err := catalog.refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	before := catalog.positions[filepath.Base(path)].offset
	cache := filepath.Join(dir, "channel-catalog.json")
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	appendChannelCapture(t, path, debug.PhaseUpstreamResponse, "same", channelSSE("fireworks", "upstream/rotation"))
	if err := catalog.refresh(context.Background(), dir); err == nil {
		t.Fatal("cache write should fail")
	}
	if catalog.positions[filepath.Base(path)].offset != before || len(catalog.snapshot(dir).Channels["upstream/rotation"].Actual) != 0 {
		t.Fatal("failed write advanced cursor or published data")
	}
	if err := os.Remove(cache); err != nil {
		t.Fatal(err)
	} // Empty test-owned directory.
	if err := catalog.refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if catalog.snapshot(dir).Error != "" {
		t.Fatal("successful retry retained error")
	}
	// Same inode, shorter file: scan from zero instead of keeping the old offset.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	appendChannelCapture(t, path, debug.PhaseUpstreamResponse, "same", channelSSE("novita", "upstream/rotation"))
	if err := catalog.refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	// Replaced inode at the same pathname must also restart at zero.
	replacement := filepath.Join(dir, "replacement.jsonl")
	appendChannelCapture(t, replacement, debug.PhaseUpstreamResponse, "same", channelSSE("alibaba", "upstream/rotation"))
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if err := catalog.refresh(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if got := catalog.snapshot(dir).Channels["upstream/rotation"].Actual; !reflect.DeepEqual(got, []string{"alibaba", "fireworks", "novita"}) {
		t.Fatalf("rotation metadata = %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := catalog.refresh(ctx, dir); err != context.Canceled {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestChannelResponseCanonicalAndAmbiguity(t *testing.T) {
	for _, tc := range []struct{ body, model string }{
		{`{"provider_metadata":{"gateway":{"routing":{"canonicalSlug":"upstream/test","finalProvider":"deepseek"}}}}`, "upstream/test"},
		{`{"provider_metadata":{"gateway":{"routing":{"finalProvider":"deepseek"}}}}`, ""},
		{"data: {\"model\":\"upstream/first\"}\n" + channelSSE("fireworks", "upstream/second"), ""},
	} {
		found := make(channelData)
		if err := readChannelResponse(tc.body, found); err != nil {
			t.Fatal(err)
		}
		if tc.model == "" {
			if len(found) != 0 {
				t.Fatal("ambiguous or absent model became evidence")
			}
		} else if !reflect.DeepEqual(mergeChannels(nil, found)[tc.model].Actual, []string{"deepseek"}) {
			t.Fatal("canonical model fallback missing")
		}
	}
}
