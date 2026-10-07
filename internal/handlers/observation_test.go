package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/storage"
)

func TestFailureWithoutUsageAtEntry(t *testing.T) {
	for _, tc := range []struct {
		endpoint         string
		cancelAfterError bool
	}{{"/v1/messages", false}, {"/v1/responses", false}, {"/v1/messages", true}, {"/v1/responses", true}} {
		name := tc.endpoint
		if tc.cancelAfterError {
			name += "/cancel-after-error"
		}
		t.Run(name, func(t *testing.T) {
			endpoint := tc.endpoint
			h, db := newResponsesTestHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"synthetic\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"synthetic\"}}\n\n"+
					"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"+
					"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"partial\"}}\n\n"+
					"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"message\":\"synthetic failure\"}}\n\n")
			}))
			routeNativeCommandCode(h)
			payload := `{"model":"m","stream":true,"max_tokens":8,"messages":[{"role":"user","content":"hello"}]}`
			if endpoint == "/v1/responses" {
				payload = `{"model":"m","stream":true,"input":"hello"}`
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(payload)).WithContext(ctx)
			w := &cancelAfterErrorWriter{ResponseRecorder: httptest.NewRecorder()}
			if tc.cancelAfterError {
				w.cancel = cancel
			}
			if endpoint == "/v1/messages" {
				h.HandleMessages(w, r)
			} else {
				h.HandleResponses(w, r)
			}
			if !strings.Contains(w.Body.String(), "partial") || !strings.Contains(w.Body.String(), "error") {
				t.Fatalf("failed stream fixture did not reach entry: %s", w.Body.String())
			}
			if tc.cancelAfterError && ctx.Err() == nil {
				t.Fatal("downstream cancellation was not exercised")
			}
			snapshot := h.metrics.GetSnapshot()
			if snapshot.RequestsReceived != 1 || snapshot.RequestsFailed != 1 || snapshot.RequestsSuccess != 0 || snapshot.UpstreamCalls != 1 {
				t.Errorf("known failure must count once, independently of usage: %+v", snapshot)
			}
			_, total, err := storage.NewRequests(db).Query(storage.RequestQuery{Page: 1, PageSize: 10})
			if err != nil || total != 0 {
				t.Fatalf("missing usage must not create a synthetic bill: rows=%d err=%v", total, err)
			}
		})
	}
}

type cancelAfterErrorWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *cancelAfterErrorWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(b)
	if w.cancel != nil && strings.Contains(string(b), "synthetic failure") {
		w.cancel()
	}
	return n, err
}
