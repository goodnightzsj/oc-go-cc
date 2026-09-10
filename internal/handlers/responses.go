package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// HandleResponses exposes the supported stateless Responses subset through the
// same routing, cancellation, fallback and accounting owner as /v1/messages.
func (h *MessagesHandler) HandleResponses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeResponsesError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	admitted, allowed := h.admitRequest(w, r)
	if !allowed {
		writeResponsesError(w, http.StatusTooManyRequests, "rate limited")
		return
	}
	r = admitted
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAnthropicBodySize))
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeResponsesError(w, status, "invalid or oversized request body")
		return
	}
	message, err := transformer.ResponsesToMessageRequest(body)
	if err != nil {
		writeResponsesError(w, http.StatusBadRequest, err.Error())
		return
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		writeResponsesError(w, http.StatusInternalServerError, "failed to encode adapted request")
		return
	}
	adapted := r.Clone(r.Context())
	adapted.URL.Path = "/v1/messages"
	adapted.Body = io.NopCloser(bytes.NewReader(encoded))
	adapted.ContentLength = int64(len(encoded))
	adapted.Header.Set("Content-Type", "application/json")
	adapted.Header.Del("Content-Length")
	bridge := &responsesHTTPWriter{
		ResponseWriter: w, headers: make(http.Header), model: message.Model,
		streaming: message.Stream != nil && *message.Stream,
	}
	h.HandleMessages(bridge, adapted)
	if r.Context().Err() != nil {
		return
	}
	if err := bridge.Finish(); err != nil {
		h.logger.Warn("Responses output adaptation failed", "error", err)
	}
}

type responsesHTTPWriter struct {
	http.ResponseWriter
	mu        sync.Mutex
	headers   http.Header
	status    int
	committed bool
	streaming bool
	model     string
	body      bytes.Buffer
	stream    *transformer.ResponsesStreamWriter
	finished  bool
	finishErr error
}

func (w *responsesHTTPWriter) Header() http.Header { return w.headers }

func (w *responsesHTTPWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *responsesHTTPWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = status
	}
}

func (w *responsesHTTPWriter) commit(status int, contentType string) {
	if w.committed {
		return
	}
	w.committed = true
	copyHeader(w.ResponseWriter.Header(), w.headers)
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.Header().Set("Content-Type", contentType)
	w.ResponseWriter.WriteHeader(status)
}

func (w *responsesHTTPWriter) isSSE() bool {
	return w.streaming && w.status < 400 && strings.HasPrefix(w.headers.Get("Content-Type"), "text/event-stream")
}

func (w *responsesHTTPWriter) startStream() {
	if w.stream == nil {
		w.commit(http.StatusOK, "text/event-stream")
		w.stream = transformer.NewResponsesStreamWriter(w.ResponseWriter, w.model)
	}
}

func (w *responsesHTTPWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.finished {
		return 0, io.ErrClosedPipe
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.isSSE() {
		w.startStream()
		return w.stream.Write(p)
	}
	return w.body.Write(p)
}

func (w *responsesHTTPWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flush()
}

func (w *responsesHTTPWriter) flush() {
	if w.isSSE() {
		w.startStream()
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// Finish is shared with MessagesHandler so adaptation failures are accounted
// once, before a request is recorded. The outer HTTP handler can call it again.
func (w *responsesHTTPWriter) Finish() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.finished {
		w.finished = true
		w.finishErr = w.finish()
	}
	return w.finishErr
}

func (w *responsesHTTPWriter) finish() error {
	if w.stream != nil {
		err := w.stream.Finish()
		w.flush()
		return err
	}
	if w.status >= 400 {
		var envelope struct {
			Error *types.APIError `json:"error"`
		}
		message := strings.TrimSpace(w.body.String())
		if json.Unmarshal(w.body.Bytes(), &envelope) == nil && envelope.Error != nil {
			message = envelope.Error.Message
		}
		copyHeader(w.ResponseWriter.Header(), w.headers)
		writeResponsesError(w.ResponseWriter, w.status, message)
		return nil
	}
	response, err := transformer.AnthropicMessageToResponse(w.body.Bytes(), w.model)
	if err != nil {
		copyHeader(w.ResponseWriter.Header(), w.headers)
		writeResponsesError(w.ResponseWriter, http.StatusBadGateway, err.Error())
		return err
	}
	w.commit(http.StatusOK, "application/json")
	if err := json.NewEncoder(w.ResponseWriter).Encode(response); err != nil {
		return fmt.Errorf("write Responses output: %w", err)
	}
	return nil
}

func writeResponsesError(w http.ResponseWriter, status int, message string) {
	kind := "invalid_request_error"
	if status == http.StatusTooManyRequests {
		kind = "rate_limit_error"
	} else if status >= 500 {
		kind = "api_error"
	}
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"type": kind, "message": message, "param": nil, "code": nil,
	}})
}
