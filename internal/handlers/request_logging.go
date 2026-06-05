package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

const payloadLogChunkSize = 16 * 1024

var redactedRequestHeaders = map[string]struct{}{
	"authorization":       {},
	"cookie":              {},
	"proxy-authorization": {},
	"set-cookie":          {},
	"x-api-key":           {},
}

func (h *MessagesHandler) requestLoggingEnabled() bool {
	cfg := h.atomic.Get()
	return cfg != nil && cfg.Logging.Requests
}

func (h *MessagesHandler) logInboundRequest(
	logger *slog.Logger,
	r *http.Request,
	clientIP string,
	rawBody json.RawMessage,
) {
	if !h.requestLoggingEnabled() {
		return
	}

	logger.Info("request metadata",
		"client", clientIP,
		"method", r.Method,
		"path", r.URL.Path,
		"headers", marshalHeadersForLog(r.Header),
		"size_bytes", len(rawBody),
	)
	h.logPayloadChunks(logger, "request body", "request_body", string(rawBody), "size_bytes", len(rawBody))
}

func (h *MessagesHandler) logOutboundResponse(
	logger *slog.Logger,
	statusCode int,
	headers http.Header,
	body string,
	streaming bool,
	outcome string,
) {
	if !h.requestLoggingEnabled() {
		return
	}

	logger.Info("response metadata",
		"status", statusCode,
		"streaming", streaming,
		"outcome", outcome,
		"headers", marshalHeadersForLog(headers),
		"size_bytes", len(body),
	)
	h.logPayloadChunks(
		logger,
		"response body",
		"response_body",
		body,
		"status", statusCode,
		"streaming", streaming,
		"outcome", outcome,
		"size_bytes", len(body),
	)
}

func (h *MessagesHandler) logPayloadChunks(
	logger *slog.Logger,
	message string,
	field string,
	payload string,
	attrs ...any,
) {
	if payload == "" {
		args := make([]any, 0, len(attrs)+4)
		args = append(args, attrs...)
		args = append(args, "part", 1, "parts", 1, field, "")
		logger.Info(message, args...)
		return
	}

	totalParts := (len(payload) + payloadLogChunkSize - 1) / payloadLogChunkSize
	for part := 0; part < totalParts; part++ {
		start := part * payloadLogChunkSize
		end := min(len(payload), start+payloadLogChunkSize)

		args := make([]any, 0, len(attrs)+6)
		args = append(args, attrs...)
		args = append(args,
			"part", part+1,
			"parts", totalParts,
			field, payload[start:end],
		)
		logger.Info(message, args...)
	}
}

func marshalHeadersForLog(headers http.Header) string {
	if len(headers) == 0 {
		return "{}"
	}

	sanitized := make(map[string][]string, len(headers))
	for key, values := range headers {
		if _, ok := redactedRequestHeaders[strings.ToLower(key)]; ok {
			sanitized[key] = []string{"***redacted***"}
			continue
		}
		sanitized[key] = append([]string(nil), values...)
	}

	body, err := json.Marshal(sanitized)
	if err != nil {
		return `{"error":"failed to marshal headers"}`
	}
	return string(body)
}
