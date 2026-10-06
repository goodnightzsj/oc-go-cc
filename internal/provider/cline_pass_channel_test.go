package provider

import (
	"bytes"
	"context"
	"encoding/json"
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
		for _, target := range []string{"deepseek", "zai", "z-ai"} {
			t.Run(target+"/stream="+map[bool]string{true: "true", false: "false"}[stream], func(t *testing.T) {
				var got map[string]json.RawMessage
				p, _ := clinePassTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
					got = nil // Decoder otherwise retains keys absent from the next request.
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				})
				model := config.ModelConfig{Provider: "cline-pass", ModelID: "cline-pass/deepseek-v4.1-flash"}
				call := func() {
					t.Helper()
					if stream {
						body, err := p.Stream(context.Background(), clinePassRequest(), model)
						if err != nil {
							t.Fatal(err)
						}
						_, err = io.Copy(io.Discard, body)
						if err != nil {
							t.Fatal(err)
						}
						if err := body.Close(); err != nil {
							t.Fatal(err)
						}
					} else if _, err := p.Execute(context.Background(), clinePassRequest(), model); err != nil {
						t.Fatal(err)
					}
				}
				// Reuse the running provider to prove settings changes apply to the
				// next request, not just to construction of a new client.
				for _, enabled := range []bool{false, true, false} {
					cfg := *p.atomic.Get()
					cfg.ClinePass.ChannelPinEnabled, cfg.ClinePass.ChannelPin = enabled, target
					p.atomic.ApplyLoaded(&cfg)
					call()
					if string(got["model"]) != `"cline-pass/deepseek-v4.1-flash"` || string(got["stream"]) != "true" {
						t.Fatal("pinning changed the billing pool or upstream stream flag")
					}
					if !enabled {
						if got["provider"] != nil || got["providerOptions"] != nil {
							t.Fatal("disabled pin emitted routing fields")
						}
						continue
					}
					direct, planner := target, target
					if target == "zai" || target == "z-ai" {
						direct, planner = "z-ai", "zai"
					}
					if string(got["provider"]) != `{"only":["`+direct+`"]}` || string(got["providerOptions"]) != `{"gateway":{"only":["`+planner+`"]}}` {
						t.Fatalf("wrong routing fields: %s / %s", got["provider"], got["providerOptions"])
					}
				}
			})
		}
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
