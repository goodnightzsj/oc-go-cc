package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/routatic/proxy/internal/config"
)

func TestClinePassChannelPinBothExecutionModes(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmtStream(stream), func(t *testing.T) {
			calls := 0
			p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				var got map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
				}
				if got["provider"] != nil || got["providerOptions"] != nil {
					t.Error("automatic route emitted routing fields")
				}
				if string(got["model"]) != `"cline-pass/deepseek-v4.1-flash"` || string(got["stream"]) != "true" {
					t.Error("model or upstream stream flag changed")
				}
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			})
			model := config.ModelConfig{Provider: p.Name(), ModelID: "cline-pass/deepseek-v4.1-flash"}
			for _, enabled := range []bool{false, true, false} {
				cfg := *p.atomic.Get()
				cfg.ClinePass.ChannelPinEnabled, cfg.ClinePass.ChannelPin = enabled, "deepseek"
				p.atomic.ApplyLoaded(&cfg)
				var err error
				if stream {
					var body io.ReadCloser
					body, err = p.Stream(t.Context(), clinePassRequest(), model)
					if body != nil {
						_, err = io.Copy(io.Discard, body)
						_ = body.Close()
					}
				} else {
					_, err = p.Execute(t.Context(), clinePassRequest(), model)
				}
				if (err != nil) != enabled {
					t.Fatalf("enabled=%t err=%v", enabled, err)
				}
			}
			if calls != 2 {
				t.Fatalf("required pin must fail locally; calls=%d", calls)
			}
		})
	}
}

func TestClinePassChannelObservation(t *testing.T) {
	metadata := `{"provider_metadata":{"gateway":{"routing":{"finalProvider":"deepseek"}}}}`
	stop := "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"
	for _, tc := range []struct{ name, target, frame, ending, status, actual string }{
		{"late delta", "deepseek", `{"choices":[{"delta":` + metadata + `}]}`, "data: [DONE]\n\n", "matched", "deepseek"},
		{"mismatch is not success", "alibaba", metadata, "data: [DONE]\n\n", "mismatch", "deepseek"},
		{"message", "deepseek", `{"choices":[{"message":` + metadata + `}]}`, "data: [DONE]\n\n", "matched", "deepseek"},
		{"missing", "deepseek", `{"choices":[]}`, "data: [DONE]\n\n", "unverified", ""},
		{"content is not metadata", "deepseek", `{"choices":[{"delta":{"content":"finalProvider: deepseek"}}]}`, "data: [DONE]\n\n", "unverified", ""},
		{"interrupted", "deepseek", metadata, "", "unverified", "deepseek"},
		{"alias", "z-ai", strings.ReplaceAll(metadata, "deepseek", "zai"), "data: [DONE]\n\n", "matched", "zai"},
		{"conflicting spellings", "deepseek", `{"provider_metadata":{"gateway":{"routing":{"finalProvider":"deepseek","resolvedProvider":"private"}}}}`, "data: [DONE]\n\n", "unverified", ""},
		{"late conflict clears partial channel", "deepseek", metadata + "\n\ndata: " + `{"provider_metadata":{"gateway":{"routing":{"finalProvider":"deepseek","resolvedProvider":"private"}}}}`, "data: [DONE]\n\n", "unverified", ""},
		{"equal spellings", "deepseek", `{"provider_metadata":{"gateway":{"routing":{"finalProvider":"deepseek","resolvedProvider":"deepseek"}}}}`, "data: [DONE]\n\n", "matched", "deepseek"},
		{"equivalent spellings", "zai", `{"provider_metadata":{"gateway":{"routing":{"finalProvider":"z-ai","resolvedProvider":"zai"}}}}`, "data: [DONE]\n\n", "matched", "z-ai"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			wire := stop + "data: " + tc.frame + "\r\n\r\n" + tc.ending
			body := newClineChannelBody(io.NopCloser(iotest.OneByteReader(strings.NewReader(wire))), tc.target, "synthetic-request")
			got, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			if err := body.Close(); err != nil {
				t.Fatal(err)
			}
			if string(got) != wire {
				t.Fatal("observation changed SSE bytes")
			}
			var entry struct {
				Adherence string `json:"adherence"`
				Actual    string `json:"actual_provider"`
			}
			if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
				t.Fatalf("expected one structured log: %v", err)
			}
			if entry.Adherence != tc.status || entry.Actual != tc.actual {
				t.Fatalf("observation = %+v, want %s/%s", entry, tc.status, tc.actual)
			}
		})
	}
}

func TestClinePassChannelMultilineFinalRouting(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		t.Run(fmt.Sprintf("%q", ending), func(t *testing.T) {
			initial := "data: {\"model\":\"vmc/synthetic\",\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"deepseek\",\"canonicalSlug\":\"deepseek/early\"}}}}\n\n"
			final := "data: {\"choices\":[{\"delta\":{\"provider_metadata\":{\n" +
				"data: \"gateway\":{\"routing\":{\"finalProvider\":\"private\",\"canonicalSlug\":\"vmc/final\"}}}}}]}\n\n"
			wire := "\xef\xbb\xbf" + strings.ReplaceAll(initial+final+"data: [DONE]\n\n", "\n", ending)
			b := newClineChannelBody(io.NopCloser(iotest.OneByteReader(strings.NewReader(wire))), "", "synthetic")
			got, err := io.ReadAll(b)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			if string(got) != wire {
				t.Fatal("observer changed source bytes")
			}
			if b.UpstreamChannel() != "private" || b.UpstreamModel() != "vmc/final" || b.ServedModel() != "vmc/synthetic" {
				t.Fatalf("channel=%s canonical=%s response=%s", b.UpstreamChannel(), b.UpstreamModel(), b.ServedModel())
			}
		})
	}
}

func TestClinePassChannelObservationPreservesReadError(t *testing.T) {
	body := newClineChannelBody(io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF)), "deepseek", "synthetic")
	_, err := io.ReadAll(body)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("read error changed: %v", err)
	}
	_ = body.Close()
}

func TestClinePassChannelOversizedLineIsUnverified(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	wire := "data: {\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"deepseek\"}}}}\n\n" +
		"data: {\"padding\":\"" + strings.Repeat("x", 1<<20) + "\",\"provider_metadata\":{\"gateway\":{\"routing\":{\"finalProvider\":\"alibaba\"}}}}\n\n" +
		"data: [DONE]\n\n"
	body := newClineChannelBody(io.NopCloser(strings.NewReader(wire)), "deepseek", "synthetic")
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if string(got) != wire {
		t.Fatal("oversized line changed the stream")
	}
	var entry struct {
		Adherence string `json:"adherence"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Adherence != "unverified" {
		t.Fatalf("lost metadata reported %q", entry.Adherence)
	}
}

type clineBlockingBody struct{ started, closed chan struct{} }

func (b *clineBlockingBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *clineBlockingBody) Close() error { close(b.closed); return nil }

func TestClinePassChannelCloseUnblocksRead(t *testing.T) {
	source := &clineBlockingBody{make(chan struct{}), make(chan struct{})}
	body := newClineChannelBody(source, "deepseek", "synthetic")
	result := make(chan error, 1)
	go func() { _, err := body.Read(make([]byte, 8)); result <- err }()
	<-source.started
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != io.ErrClosedPipe {
			t.Fatalf("read error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock Read")
	}
}
