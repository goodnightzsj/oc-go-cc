package transformer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/sse"
	"github.com/routatic/proxy/pkg/types"
)

// ErrClientDisconnected is returned when the client disconnects during streaming.
var ErrClientDisconnected = fmt.Errorf("client disconnected")

// ErrStreamIdle is returned when no bytes arrive within idleTimeout on the
// upstream stream. The connection is stale (e.g. backend hang or network
// partition). The handler decides whether to fall back to another model.
var ErrStreamIdle = fmt.Errorf("upstream stream idle")

var ErrEmptyStream = fmt.Errorf("upstream returned empty stream")

const thinkingSignaturePlaceholder = "proxy-thinking-placeholder"

// readBufPool pools read buffers for streaming operations.
// sync.Pool reduces GC pressure under concurrent stream load by reusing
// 4KB buffers across goroutines instead of allocating fresh ones per read.
// Pool stores pointers to slices to avoid allocation on Put (SA6002).
var readBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 4096)
		return &b
	},
}

// IsIdleTimeout reports whether err is a read-timeout (network deadline
// exceeded on an otherwise live stream).
func IsIdleTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// StreamHandler handles streaming SSE transformation from OpenAI to Anthropic format.
type StreamHandler struct {
	responseTransformer *ResponseTransformer
}

// NewStreamHandler creates a new stream handler.
func NewStreamHandler() *StreamHandler {
	return &StreamHandler{
		responseTransformer: NewResponseTransformer(),
	}
}

// ProxyStream takes an OpenAI streaming response and writes Anthropic-format SSE to the writer.
// It reads OpenAI ChatCompletionChunk SSE events and transforms them into Anthropic MessageEvent SSE events.
// The streamCtx is the per-model attempt context (carries streaming_timeout_ms); the caller
// should wrap openaiResp with NewCtxReadCloser so the body read also respects the deadline.
//
// Reads are incremental; only the current SSE event is buffered for decoding.
//
// idleTimeout is the maximum gap between bytes on the upstream stream. The
// stream lives as long as data keeps flowing; only an idle period longer than
// idleTimeout is treated as a stuck connection and surfaces as ErrStreamIdle.
// Pass 0 to disable (stream lives until EOF or error).
func (h *StreamHandler) ProxyStream(
	w http.ResponseWriter,
	openaiResp io.ReadCloser,
	originalModel string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	defer func() { _ = openaiResp.Close() }()
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by response writer")
	}

	// Generate a unique message ID for this stream.
	msgID := "msg_" + generateID()

	// Send message_start event with the full message envelope.
	msgStart := types.MessageEvent{
		Type: "message_start",
		Message: &types.MessageResponse{
			ID:      msgID,
			Type:    "message",
			Role:    "assistant",
			Content: []types.ContentBlock{},
			Model:   originalModel,
		},
	}
	if err := writeSSEEvent(w, msgStart); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	contentIndex := 0
	var decoder sse.Decoder
	contentStarted := false
	reasoningStarted := false
	sawRefusal := false
	terminalStopReason := ""
	var terminalUsage *types.UsageInfo
	startedToolCalls := make(map[int]*chatToolState)

	// Get a buffer from the pool; return it when done.
	readBuf := readBufPool.Get().(*[]byte)
	defer readBufPool.Put(readBuf)

	// Start the idle watchdog. Each successful read pings the watchdog so
	// the stream lives as long as data keeps flowing. If no bytes arrive
	// within idleTimeout, cancel() is called, which aborts the upstream
	// HTTP request and causes the next Read to return a context error.
	ping := StartIdleWatchdog(clientCtx, cancel, idleTimeout)

	for {
		// Check if client disconnected
		select {
		case <-clientCtx.Done():
			return ErrClientDisconnected
		default:
		}

		// Read chunk from upstream
		n, err := openaiResp.Read(*readBuf)
		if n > 0 {
			// Data is flowing — reset the idle watchdog so the stream
			// lives as long as data keeps arriving.
			ping()
			if err := decoder.Feed((*readBuf)[:n], func(event string, data []byte) error {
				return h.processChatEvent(w, flusher, event, data, &contentIndex, &contentStarted, &reasoningStarted, &sawRefusal, &terminalStopReason, &terminalUsage, startedToolCalls)
			}); err != nil {
				return err
			}
		}

		if err == io.EOF {
			if err := decoder.Finish(); err != nil {
				return err
			}
			break
		}
		if err != nil {
			if IsIdleTimeout(err) {
				return ErrStreamIdle
			}
			// When the idle watchdog fires, it cancels the upstream context
			// which produces context.Canceled on Read. Distinguish that
			// from a client disconnect by checking clientCtx.
			if (errors.Is(err, context.Canceled) || errors.Is(err, ErrStreamReadCanceled)) && clientCtx.Err() == nil {
				return ErrStreamIdle
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	if terminalStopReason == "" {
		return fmt.Errorf("chat stream has no finish reason: %w", io.ErrUnexpectedEOF)
	}
	if terminalStopReason == "end_turn" && sawRefusal {
		terminalStopReason = "refusal"
	}

	// Anthropic expects one terminal message_delta containing both stop_reason
	// and usage. OpenAI-compatible providers commonly send those in separate
	// chunks, so both are retained until the upstream stream ends.
	msgDelta := types.MessageEvent{
		Type: "message_delta",
		Delta: &types.Delta{
			StopReason: terminalStopReason,
		},
		Usage: ChatUsageToAnthropic(terminalUsage),
	}
	if err := writeSSEEvent(w, msgDelta); err != nil {
		return ErrClientDisconnected
	}

	// Send message_stop event to signal stream completion.
	stopEvent := types.MessageEvent{
		Type: "message_stop",
	}
	if err := writeSSEEvent(w, stopEvent); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	return nil
}

// processChatEvent processes the joined data of one complete upstream SSE event.
func (h *StreamHandler) processChatEvent(
	w http.ResponseWriter,
	flusher http.Flusher,
	eventType string,
	data []byte,
	contentIndex *int,
	contentStarted *bool,
	reasoningStarted *bool,
	sawRefusal *bool,
	terminalStopReason *string,
	terminalUsage **types.UsageInfo,
	startedToolCalls map[int]*chatToolState,
) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}

	// Handle [DONE] marker
	if bytes.Equal(data, []byte("[DONE]")) {
		return nil
	}

	// Unmarshal updates reported fields only; a later partial usage must not
	// erase earlier totals or breakdowns. Explicit zero still overwrites.
	previousReasoning := reasoningTokensOf(*terminalUsage)
	var chunk types.ChatCompletionChunk
	if *terminalUsage != nil {
		next := **terminalUsage
		// Decode aliases together for this event: a fresh alias must outrank
		// an older canonical split, but not a canonical split in the same event.
		next.CompletionTokensDetails, next.CompletionThinkingTokens = nil, 0
		chunk.Usage = &next
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return fmt.Errorf("invalid Chat Completions SSE: %w", err)
	}
	observeSourceModel(w, chunk.Model)
	if chunk.Usage != nil {
		if reasoningTokensOf(chunk.Usage) == 0 {
			chunk.Usage.CompletionThinkingTokens = previousReasoning
		}
		*terminalUsage = chunk.Usage
		// Preserve reported usage even when the terminal event never arrives.
		if uw, ok := w.(interface {
			SetPartialUsage(*types.Usage)
		}); ok {
			uw.SetPartialUsage(ChatUsageToAnthropic(chunk.Usage))
		}
	}

	if err := upstreamJSONError("Chat Completions", chunk.Error); err != nil {
		return err
	}
	if eventType == "error" {
		return upstreamJSONError("Chat Completions", data)
	}
	if len(chunk.Choices) == 0 {
		return nil
	}

	choice := chunk.Choices[0]
	textContent := choice.Delta.ContentText()
	if choice.Delta.Refusal != nil && *choice.Delta.Refusal != "" {
		textContent += *choice.Delta.Refusal
		*sawRefusal = true
	}
	if *terminalStopReason != "" && (textContent != "" || choice.Delta.ReasoningText() != "") {
		return fmt.Errorf("chat content arrived after finish_reason")
	}

	// Handle reasoning content deltas
	if reasoning := choice.Delta.ReasoningText(); reasoning != "" {
		if !*reasoningStarted {
			// If text was already started, close it first
			if *contentStarted {
				stopEvent := types.MessageEvent{
					Type:  "content_block_stop",
					Index: contentIndex,
				}
				if err := writeSSEEvent(w, stopEvent); err != nil {
					return ErrClientDisconnected
				}
				*contentIndex++
				*contentStarted = false
			}
			*reasoningStarted = true
			startEvent := types.MessageEvent{
				Type:         "content_block_start",
				Index:        contentIndex,
				ContentBlock: &types.ContentBlock{Type: "thinking", Thinking: "", Signature: thinkingSignaturePlaceholder},
			}
			if err := writeSSEEvent(w, startEvent); err != nil {
				return ErrClientDisconnected
			}
		}

		delta := types.Delta{
			Type:     "thinking_delta",
			Thinking: reasoning,
		}
		event := types.MessageEvent{
			Type:  "content_block_delta",
			Index: contentIndex,
			Delta: &delta,
		}
		if err := writeSSEEvent(w, event); err != nil {
			return ErrClientDisconnected
		}
		flusher.Flush()
	}

	// Handle text content deltas
	if textContent != "" {
		if !*contentStarted {
			// If reasoning was already started, close it first
			if *reasoningStarted {
				if err := writeThinkingBlockStop(w, *contentIndex); err != nil {
					return ErrClientDisconnected
				}
				*contentIndex++
				*reasoningStarted = false
			}
			*contentStarted = true
			startEvent := types.MessageEvent{
				Type:         "content_block_start",
				Index:        contentIndex,
				ContentBlock: &types.ContentBlock{Type: "text", Text: ""},
			}
			if err := writeSSEEvent(w, startEvent); err != nil {
				return ErrClientDisconnected
			}
		}

		delta := types.Delta{
			Type: "text_delta",
			Text: textContent,
		}
		event := types.MessageEvent{
			Type:  "content_block_delta",
			Index: contentIndex,
			Delta: &delta,
		}
		if err := writeSSEEvent(w, event); err != nil {
			return ErrClientDisconnected
		}
		flusher.Flush()
	}

	// Handle tool call deltas.
	// OpenAI streams tool calls incrementally: the first chunk for a given
	// tool call carries id + name (+ possibly empty arguments), subsequent
	// chunks carry only incremental arguments.  We must create exactly one
	// content_block_start per tool call, then stream deltas for it.
	if len(choice.Delta.ToolCalls) > 0 {
		for _, tc := range choice.Delta.ToolCalls {
			oi := tc.Index // OpenAI tool_calls array index

			tool, exists := startedToolCalls[oi]
			if !exists {
				if tc.Function.Name == "" {
					// Ghost chunk: this index was closed and recycled, but
					// has no name/id. Ignore — the real tool call was
					// already fully processed.
					continue
				}
				if *terminalStopReason != "" {
					return fmt.Errorf("chat tool arrived after finish_reason")
				}
				// Close any existing content/reasoning block before opening the
				// tool block.  Capture the state first so we know whether to
				// advance contentIndex — the close itself clears the flags.
				hadStartedBlock := *contentStarted || *reasoningStarted
				if hadStartedBlock {
					var err error
					if *reasoningStarted {
						err = writeThinkingBlockStop(w, *contentIndex)
					} else {
						err = writeContentBlockStop(w, *contentIndex)
					}
					if err != nil {
						return ErrClientDisconnected
					}
					*contentStarted = false
					*reasoningStarted = false
				}
				// First time seeing this logical tool call — start a new block.
				// Only increment contentIndex when a previous text or reasoning
				// block was already started, OR when a prior tool call has already
				// claimed index 0 (parallel or sequential tool calls).  If nothing
				// was started yet (single-tool response), the first tool block
				// keeps contentIndex at 0 so the Anthropic SSE content block
				// indices are contiguous.
				if hadStartedBlock || len(startedToolCalls) > 0 {
					*contentIndex++
				}
				tool = &chatToolState{index: *contentIndex}
				startedToolCalls[oi] = tool

				toolID := tc.ID
				if toolID == "" {
					toolID = fmt.Sprintf("toolu_%s", generateID())
				}
				startEvent := types.MessageEvent{
					Type:  "content_block_start",
					Index: &tool.index,
					ContentBlock: &types.ContentBlock{
						Type:  "tool_use",
						ID:    toolID,
						Name:  tc.Function.Name,
						Input: json.RawMessage(`{}`),
					},
				}
				if err := writeSSEEvent(w, startEvent); err != nil {
					return ErrClientDisconnected
				}
			}

			// Send argument delta (if any) — whether new or continuation.
			if tc.Function.Arguments != "" {
				if len(tool.arguments)+len(tc.Function.Arguments) > 1<<20 {
					return fmt.Errorf("chat tool arguments exceed 1 MiB")
				}
				tool.arguments += tc.Function.Arguments
				delta := types.Delta{
					Type:        "input_json_delta",
					PartialJSON: tc.Function.Arguments,
				}
				event := types.MessageEvent{
					Type:  "content_block_delta",
					Index: &tool.index,
					Delta: &delta,
				}
				if err := writeSSEEvent(w, event); err != nil {
					return ErrClientDisconnected
				}
			}
			flusher.Flush()
		}
	}

	// Handle finish reason
	if choice.FinishReason != "" {
		for _, tool := range startedToolCalls {
			if choice.FinishReason != "length" && tool.arguments != "" && !json.Valid([]byte(tool.arguments)) {
				return fmt.Errorf("incomplete Chat Completions function arguments")
			}
		}
		// Close any open content block (reasoning or text)
		if *reasoningStarted {
			if err := writeThinkingBlockStop(w, *contentIndex); err != nil {
				return ErrClientDisconnected
			}
			*reasoningStarted = false
		} else if *contentStarted {
			if err := writeContentBlockStop(w, *contentIndex); err != nil {
				return ErrClientDisconnected
			}
			*contentStarted = false
		}

		// Close any open tool_use blocks in ascending index order.
		if len(startedToolCalls) > 0 {
			indices := make([]int, 0, len(startedToolCalls))
			for _, tool := range startedToolCalls {
				indices = append(indices, tool.index)
			}
			slices.Sort(indices)
			for _, idx := range indices {
				stopEvent := types.MessageEvent{
					Type:  "content_block_stop",
					Index: &idx,
				}
				if err := writeSSEEvent(w, stopEvent); err != nil {
					return ErrClientDisconnected
				}
			}
			clear(startedToolCalls)
		}

		stopReason, err := h.responseTransformer.mapFinishReason(choice.FinishReason)
		if err != nil {
			return err
		}
		*terminalStopReason = stopReason
		flusher.Flush()
	}

	return nil
}

type chatToolState struct {
	index     int
	arguments string
}

// ChatUsageToAnthropic preserves absence and maps cache/reasoning counts once.
func ChatUsageToAnthropic(usage *types.UsageInfo) *types.Usage {
	if usage == nil {
		return nil
	}
	in, cacheRead, cacheCreate := splitPromptTokens(usage)
	return &types.Usage{
		InputTokens:              in,
		OutputTokens:             usage.CompletionTokens,
		CacheCreationInputTokens: cacheCreate,
		CacheReadInputTokens:     cacheRead,
		ReasoningTokens:          reasoningTokensOf(usage),
	}
}

// reasoningTokensOf reads the thinking split of the completion, under either
// spelling a gateway uses for it.
//
// The two fields are the same figure: `completion_tokens_details.reasoning_tokens`
// is the OpenAI-standard shape and `completion_thinking_tokens` is what the
// DeepSeek-family gateways publish alongside it. Reading both means a count is
// not lost to whichever one a particular upstream chose, and reading only the
// first non-zero one keeps a gateway that sends both from doubling it.
func reasoningTokensOf(usage *types.UsageInfo) int {
	if usage == nil {
		return 0
	}
	if usage.CompletionTokensDetails != nil && usage.CompletionTokensDetails.ReasoningTokens > 0 {
		return usage.CompletionTokensDetails.ReasoningTokens
	}
	return usage.CompletionThinkingTokens
}

func ResponsesUsageToAnthropic(usage *types.ResponsesUsage) *types.Usage {
	if usage == nil {
		return ChatUsageToAnthropic(nil)
	}
	return ChatUsageToAnthropic(&types.UsageInfo{
		PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens,
		PromptTokensDetails: usage.InputTokensDetails, CompletionTokensDetails: usage.OutputTokensDetails,
	})
}

func geminiUsageToAnthropic(usage *types.GeminiUsage) *types.Usage {
	if usage == nil {
		return nil
	}
	count := func(value *int) int {
		if value == nil {
			return 0
		}
		return nonNegative(*value)
	}
	// Gemini counts thoughts outside candidates, unlike Chat/Responses output.
	thinking := count(usage.ThoughtsTokenCount)
	return ChatUsageToAnthropic(&types.UsageInfo{
		PromptTokens:            count(usage.PromptTokenCount) + count(usage.ToolUsePromptTokenCount),
		CompletionTokens:        count(usage.CandidatesTokenCount) + thinking,
		PromptTokensDetails:     &types.PromptTokensDetails{CachedTokens: count(usage.CachedContentTokenCount)},
		CompletionTokensDetails: &types.CompletionTokensDetails{ReasoningTokens: thinking},
	})
}

// splitPromptTokens maps an OpenAI-compatible usage block onto Anthropic's
// (input, cache_read, cache_creation) triple.
//
// DeepSeek reports prompt_cache_hit_tokens + prompt_cache_miss_tokens ==
// prompt_tokens: the two fields *partition* the prompt rather than counting
// extra tokens on top of it. The cache-miss part is the portion actually billed
// at the full input rate, so it maps to Anthropic's `input_tokens`, and only the
// hit part is cache_read. Subtracting both (the previous behaviour) drove
// input_tokens to zero on every cached turn and, when upstream omitted the cache
// fields entirely, billed the whole prompt at the uncached rate.
func splitPromptTokens(usage *types.UsageInfo) (input, cacheRead, cacheCreate int) {
	// OpenAI-standard form (OpenCode Go oa-compat gateway): prompt_tokens is
	// the full prompt, of which prompt_tokens_details.cached_tokens were read
	// from cache. The rest is fresh input written to cache this turn.
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
		cached := usage.PromptTokensDetails.CachedTokens
		return nonNegative(usage.PromptTokens - cached), cached, 0
	}
	hit, miss := usage.PromptCacheHitTokens, usage.PromptCacheMissTokens
	if hit == 0 && miss == 0 {
		return usage.PromptTokens, 0, 0
	}
	// Partitioned form (DeepSeek): hit + miss accounts for the whole prompt.
	// An oa-compat NON-streaming response is an exception: its miss is the
	// FULL prompt (cache not stripped — observed 2026-09-02: prompt=261241,
	// hit=261120, miss=261241), which also satisfies hit+miss >= prompt and
	// would otherwise price the hit part twice (as fresh input and as cache
	// read). Detect it by miss covering the whole prompt.
	if hit+miss >= usage.PromptTokens {
		if miss >= usage.PromptTokens {
			return nonNegative(usage.PromptTokens - hit), hit, 0
		}
		return miss, hit, 0
	}
	// Additive form: cache counts sit outside prompt_tokens.
	return nonNegative(usage.PromptTokens - hit - miss), hit, miss
}

// writeContentBlockStop writes a content_block_stop SSE event at the given index.
func writeContentBlockStop(w http.ResponseWriter, index int) error {
	return writeSSEEvent(w, types.MessageEvent{
		Type:  "content_block_stop",
		Index: &index,
	})
}

func writeThinkingBlockStop(w http.ResponseWriter, index int) error {
	if err := writeSSEEvent(w, types.MessageEvent{
		Type:  "content_block_delta",
		Index: &index,
		Delta: &types.Delta{Type: "signature_delta", Signature: thinkingSignaturePlaceholder},
	}); err != nil {
		return err
	}
	return writeContentBlockStop(w, index)
}

// writeSSEEvent writes a single SSE event to the HTTP response writer.
// Format: "event: <type>\ndata: <json>\n\n"
func writeSSEEvent(w http.ResponseWriter, event types.MessageEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
	return err
}

// generateID creates a unique identifier based on current time.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ProxyResponsesStream takes an OpenAI Responses streaming response and writes Anthropic-format SSE.
// streamCtx is the per-model attempt context (carries streaming_timeout_ms); the caller should
// wrap responsesResp with NewCtxReadCloser so the body read also respects the deadline.
func (h *StreamHandler) ProxyResponsesStream(
	w http.ResponseWriter,
	responsesResp io.ReadCloser,
	originalModel string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	defer func() { _ = responsesResp.Close() }()
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by response writer")
	}

	msgID := "msg_" + generateID()
	msgStart := types.MessageEvent{
		Type: "message_start",
		Message: &types.MessageResponse{
			ID:      msgID,
			Type:    "message",
			Role:    "assistant",
			Content: []types.ContentBlock{},
			Model:   originalModel,
		},
	}
	if err := writeSSEEvent(w, msgStart); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	var decoder sse.Decoder
	state := responsesStreamState{tools: make(map[string]*responsesToolState)}
	readBuf := readBufPool.Get().(*[]byte)
	defer readBufPool.Put(readBuf)

	ping := StartIdleWatchdog(clientCtx, cancel, idleTimeout)

	for {
		select {
		case <-clientCtx.Done():
			return ErrClientDisconnected
		default:
		}

		n, err := responsesResp.Read(*readBuf)
		if n > 0 {
			ping()
			if err := decoder.Feed((*readBuf)[:n], func(event string, data []byte) error {
				return h.processResponsesSSELine(w, flusher, event, data, &state)
			}); err != nil {
				return err
			}
		}

		if err == io.EOF {
			if err := decoder.Finish(); err != nil {
				return err
			}
			break
		}
		if err != nil {
			if IsIdleTimeout(err) {
				return ErrStreamIdle
			}
			if (errors.Is(err, context.Canceled) || errors.Is(err, ErrStreamReadCanceled)) && clientCtx.Err() == nil {
				return ErrStreamIdle
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	if !state.terminal {
		return fmt.Errorf("responses stream has no terminal event: %w", io.ErrUnexpectedEOF)
	}
	if err := state.closeText(w); err != nil {
		return err
	}
	indices := make([]int, 0, len(state.tools))
	for _, tool := range state.tools {
		if state.stopReason != "max_tokens" && !json.Valid([]byte(tool.arguments)) {
			return fmt.Errorf("incomplete Responses function arguments")
		}
		if !tool.closed {
			indices = append(indices, tool.index)
		}
	}
	slices.Sort(indices)
	for _, index := range indices {
		if err := writeContentBlockStop(w, index); err != nil {
			return ErrClientDisconnected
		}
	}
	stopReason := "end_turn"
	if len(state.tools) > 0 {
		stopReason = "tool_use"
	}
	if state.stopReason != "end_turn" {
		stopReason = state.stopReason
	} else if state.refused {
		stopReason = "refusal"
	}
	if err := writeSSEEvent(w, types.MessageEvent{Type: "message_delta", Delta: &types.Delta{StopReason: stopReason}, Usage: ResponsesUsageToAnthropic(state.usage)}); err != nil {
		return ErrClientDisconnected
	}

	stopEvent := types.MessageEvent{
		Type: "message_stop",
	}
	if err := writeSSEEvent(w, stopEvent); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	return nil
}

func (h *StreamHandler) processResponsesSSELine(
	w http.ResponseWriter,
	flusher http.Flusher,
	eventType string,
	data []byte,
	state *responsesStreamState,
) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
		return nil
	}

	chunk, err := state.ReadEvent(eventType, data)
	observeSourceModel(w, state.Model)
	if recorder, ok := w.(interface{ SetPartialUsage(*types.Usage) }); ok && state.usage != nil {
		recorder.SetPartialUsage(state.ReportedUsage())
	}
	if err != nil {
		return err
	}
	if chunk == nil {
		return nil
	}
	if chunk.Type == "response.completed" || chunk.Type == "response.done" || chunk.Type == "response.incomplete" {
		if chunk.Response != nil {
			for _, item := range chunk.Response.Output {
				if item.Type == "reasoning" || item.Type == "message" {
					if err := state.visibleItem(w, item, true); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if chunk.Type == "response.refusal.delta" || chunk.Type == "response.refusal.done" {
		text, done := chunk.Delta, chunk.Type == "response.refusal.done"
		if done {
			text = chunk.Refusal
		}
		return state.visibleText(w, flusher, responsesVisibleKey{itemID: chunk.ItemID, index: chunk.ContentIndex, refusal: true}, text, done, done)
	}
	if (chunk.Type == "response.content_part.added" || chunk.Type == "response.content_part.done") && chunk.Part != nil && chunk.Part.Type == "refusal" {
		return state.visibleText(w, flusher, responsesVisibleKey{itemID: chunk.ItemID, index: chunk.ContentIndex, refusal: true}, chunk.Part.Refusal, true, chunk.Type == "response.content_part.done")
	}
	if strings.HasPrefix(chunk.Type, "response.reasoning_summary_") {
		key := responsesVisibleKey{itemID: chunk.ItemID, index: chunk.SummaryIndex}
		switch chunk.Type {
		case "response.reasoning_summary_text.delta":
			return state.visibleText(w, flusher, key, chunk.Delta, false, false)
		case "response.reasoning_summary_text.done":
			return state.visibleText(w, flusher, key, chunk.Text, true, true)
		case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
			if chunk.Part == nil || chunk.Part.Type != "summary_text" {
				return fmt.Errorf("invalid Responses reasoning summary part")
			}
			return state.visibleText(w, flusher, key, chunk.Part.Text, true, chunk.Type == "response.reasoning_summary_part.done")
		}
	}
	if chunk.Type == "response.output_text.delta" && chunk.Delta != "" {
		if !state.textOpen {
			if err := state.closeText(w); err != nil {
				return err
			}
			state.textOpen = true
			state.textIndex = state.nextIndex
			state.nextIndex++
			startEvent := types.MessageEvent{
				Type:         "content_block_start",
				Index:        &state.textIndex,
				ContentBlock: &types.ContentBlock{Type: "text", Text: ""},
			}
			if err := writeSSEEvent(w, startEvent); err != nil {
				return ErrClientDisconnected
			}
		}

		delta := types.Delta{
			Type: "text_delta",
			Text: chunk.Delta,
		}
		event := types.MessageEvent{
			Type:  "content_block_delta",
			Index: &state.textIndex,
			Delta: &delta,
		}
		if err := writeSSEEvent(w, event); err != nil {
			return ErrClientDisconnected
		}
		flusher.Flush()
	}

	if chunk.Type == "response.output_item.added" || chunk.Type == "response.output_item.done" {
		item := chunk.Item
		if item == nil && len(chunk.Output) > 0 {
			item = &chunk.Output[0]
		}
		if item == nil {
			return fmt.Errorf("responses output item is missing")
		}
		if item.Type == "reasoning" {
			return state.visibleItem(w, *item, chunk.Type == "response.output_item.done")
		}
		if item.Type == "message" && chunk.Type == "response.output_item.done" {
			if err := state.visibleItem(w, *item, true); err != nil {
				return err
			}
			return state.closeText(w)
		}
		if item.Type != "function_call" {
			return nil
		}
		tool := state.tools[item.ID]
		if tool == nil {
			if item.ID == "" || item.CallID == "" || item.Name == "" {
				return fmt.Errorf("incomplete Responses function identity")
			}
			if err := state.closeText(w); err != nil {
				return err
			}
			tool = &responsesToolState{index: state.nextIndex}
			state.nextIndex++
			state.tools[item.ID] = tool
			if err := writeSSEEvent(w, types.MessageEvent{Type: "content_block_start", Index: &tool.index, ContentBlock: &types.ContentBlock{Type: "tool_use", ID: item.CallID, Name: item.Name, Input: json.RawMessage(`{}`)}}); err != nil {
				return ErrClientDisconnected
			}
		}
		if item.Arguments != "" && item.Arguments != tool.arguments {
			if tool.closed || !bytes.HasPrefix([]byte(item.Arguments), []byte(tool.arguments)) {
				return fmt.Errorf("responses function arguments changed after streaming")
			}
			if err := tool.appendArguments(w, item.Arguments[len(tool.arguments):]); err != nil {
				return err
			}
		}
		if chunk.Type == "response.output_item.done" && !tool.closed {
			if item.Status != "incomplete" && !json.Valid([]byte(tool.arguments)) {
				return fmt.Errorf("invalid completed Responses function arguments")
			}
			if err := writeContentBlockStop(w, tool.index); err != nil {
				return ErrClientDisconnected
			}
			tool.closed = true
		}
		flusher.Flush()
	}
	if chunk.Type == "response.function_call_arguments.delta" {
		tool := state.tools[chunk.ItemID]
		if tool == nil || tool.closed {
			return fmt.Errorf("responses arguments reference an unopened function")
		}
		if err := tool.appendArguments(w, chunk.Delta); err != nil {
			return err
		}
		flusher.Flush()
	}

	return nil
}

type responsesToolState struct {
	index     int
	arguments string
	closed    bool
}

func (t *responsesToolState) appendArguments(w http.ResponseWriter, delta string) error {
	if delta == "" {
		return nil
	}
	if len(t.arguments)+len(delta) > 1<<20 {
		return fmt.Errorf("Responses function arguments exceed 1 MiB")
	}
	if err := writeSSEEvent(w, types.MessageEvent{Type: "content_block_delta", Index: &t.index, Delta: &types.Delta{Type: "input_json_delta", PartialJSON: delta}}); err != nil {
		return ErrClientDisconnected
	}
	t.arguments += delta
	return nil
}

type responsesStreamState struct {
	ResponsesObservation
	nextIndex     int
	textIndex     int
	textOpen      bool
	tools         map[string]*responsesToolState
	visibleParts  map[responsesVisibleKey]*responsesVisibleState
	activeVisible *responsesVisibleState
	refused       bool
}

func (s *responsesStreamState) closeText(w http.ResponseWriter) error {
	if s.textOpen {
		if err := writeContentBlockStop(w, s.textIndex); err != nil {
			return ErrClientDisconnected
		}
		s.textOpen = false
	}
	if s.activeVisible != nil {
		if err := writeContentBlockStop(w, s.activeVisible.index); err != nil {
			return ErrClientDisconnected
		}
		s.activeVisible.closed = true
		s.activeVisible = nil
	}
	return nil
}

type responsesVisibleKey struct {
	itemID  string
	index   int
	refusal bool
}

type responsesVisibleState struct {
	index  int
	text   strings.Builder
	closed bool
}

func (s *responsesStreamState) visibleItem(w http.ResponseWriter, item types.ResponsesOutput, done bool) error {
	if item.Type == "message" {
		for i, content := range item.Content {
			if content.Type == "refusal" {
				if err := s.visibleText(w, w.(http.Flusher), responsesVisibleKey{itemID: item.ID, index: i, refusal: true}, content.Refusal, true, done); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if item.EncryptedContent != "" {
		return fmt.Errorf("encrypted Responses reasoning requires a native Responses target")
	}
	for i, summary := range item.Summary {
		if summary.Type != "summary_text" {
			return fmt.Errorf("unsupported Responses reasoning summary %q", summary.Type)
		}
		if err := s.visibleText(w, w.(http.Flusher), responsesVisibleKey{itemID: item.ID, index: i}, summary.Text, true, done); err != nil {
			return err
		}
	}
	return nil
}

func (s *responsesStreamState) visibleText(w http.ResponseWriter, flusher http.Flusher, key responsesVisibleKey, text string, full, done bool) error {
	blockType := "thinking"
	delta := &types.Delta{Type: "thinking_delta"}
	if key.refusal {
		blockType = "text"
		delta.Type = "text_delta"
		s.refused = true
	}
	if s.visibleParts == nil {
		s.visibleParts = make(map[responsesVisibleKey]*responsesVisibleState)
	}
	part := s.visibleParts[key]
	if part == nil {
		if err := s.closeText(w); err != nil {
			return err
		}
		part = &responsesVisibleState{index: s.nextIndex}
		s.nextIndex++
		s.visibleParts[key] = part
		s.activeVisible = part
		if err := writeSSEEvent(w, types.MessageEvent{Type: "content_block_start", Index: &part.index, ContentBlock: &types.ContentBlock{Type: blockType}}); err != nil {
			return ErrClientDisconnected
		}
	}
	if full {
		if !strings.HasPrefix(text, part.text.String()) {
			return fmt.Errorf("Responses visible content changed after streaming")
		}
		text = text[part.text.Len():]
	}
	if part.closed && text != "" {
		return fmt.Errorf("Responses visible content changed after closing")
	}
	if text != "" {
		if part.text.Len()+len(text) > 1<<20 {
			return fmt.Errorf("Responses visible content exceeds 1 MiB")
		}
		if key.refusal {
			delta.Text = text
		} else {
			delta.Thinking = text
		}
		if err := writeSSEEvent(w, types.MessageEvent{Type: "content_block_delta", Index: &part.index, Delta: delta}); err != nil {
			return ErrClientDisconnected
		}
		part.text.WriteString(text)
	}
	if done && !part.closed {
		if err := s.closeText(w); err != nil {
			return err
		}
	}
	flusher.Flush()
	return nil
}

// ProxyGeminiStream takes a Gemini streaming response and writes Anthropic-format SSE.
// streamCtx is the per-model attempt context (carries streaming_timeout_ms); the caller should
// wrap geminiResp with NewCtxReadCloser so the body read also respects the deadline.
func (h *StreamHandler) ProxyGeminiStream(
	w http.ResponseWriter,
	geminiResp io.ReadCloser,
	originalModel string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	defer func() { _ = geminiResp.Close() }()
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by response writer")
	}

	msgID := "msg_" + generateID()
	msgStart := types.MessageEvent{
		Type: "message_start",
		Message: &types.MessageResponse{
			ID:      msgID,
			Type:    "message",
			Role:    "assistant",
			Content: []types.ContentBlock{},
			Model:   originalModel,
		},
	}
	if err := writeSSEEvent(w, msgStart); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	state := geminiStreamState{}
	var decoder sse.Decoder
	readBuf := readBufPool.Get().(*[]byte)
	defer readBufPool.Put(readBuf)

	ping := StartIdleWatchdog(clientCtx, cancel, idleTimeout)

	for {
		select {
		case <-clientCtx.Done():
			return ErrClientDisconnected
		default:
		}

		n, err := geminiResp.Read(*readBuf)
		if n > 0 {
			ping()
			if err := decoder.Feed((*readBuf)[:n], func(event string, data []byte) error {
				return h.processGeminiSSELine(w, flusher, event, data, &state)
			}); err != nil {
				return err
			}
		}

		if err == io.EOF {
			if err := decoder.Finish(); err != nil {
				return err
			}
			break
		}
		if err != nil {
			if IsIdleTimeout(err) {
				return ErrStreamIdle
			}
			if (errors.Is(err, context.Canceled) || errors.Is(err, ErrStreamReadCanceled)) && clientCtx.Err() == nil {
				return ErrStreamIdle
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	if state.stopReason == "" {
		return fmt.Errorf("gemini stream has no finish reason: %w", io.ErrUnexpectedEOF)
	}
	if err := state.closeBlock(w); err != nil {
		return ErrClientDisconnected
	}

	// Usage can arrive after the candidate's finish reason.
	if err := writeSSEEvent(w, types.MessageEvent{
		Type: "message_delta", Delta: &types.Delta{StopReason: state.stopReason},
		Usage: geminiUsageToAnthropic(state.usage),
	}); err != nil {
		return ErrClientDisconnected
	}

	stopEvent := types.MessageEvent{
		Type: "message_stop",
	}
	if err := writeSSEEvent(w, stopEvent); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	return nil
}

type geminiStreamState struct {
	contentIndex int
	blockType    string
	stopReason   string
	usage        *types.GeminiUsage
	hasTools     bool
}

func (s *geminiStreamState) observeUsage(next *types.GeminiUsage) {
	if s.usage == nil {
		s.usage = &types.GeminiUsage{}
	}
	if next.PromptTokenCount != nil {
		s.usage.PromptTokenCount = next.PromptTokenCount
	}
	if next.CandidatesTokenCount != nil {
		s.usage.CandidatesTokenCount = next.CandidatesTokenCount
	}
	if next.ThoughtsTokenCount != nil && (*next.ThoughtsTokenCount > 0 || s.usage.ThoughtsTokenCount == nil) {
		s.usage.ThoughtsTokenCount = next.ThoughtsTokenCount
	}
	if next.CachedContentTokenCount != nil {
		s.usage.CachedContentTokenCount = next.CachedContentTokenCount
	}
	if next.ToolUsePromptTokenCount != nil {
		s.usage.ToolUsePromptTokenCount = next.ToolUsePromptTokenCount
	}
}

func (s *geminiStreamState) closeBlock(w http.ResponseWriter) error {
	if s.blockType == "" {
		return nil
	}
	if err := writeContentBlockStop(w, s.contentIndex); err != nil {
		return err
	}
	s.contentIndex++
	s.blockType = ""
	return nil
}

func (h *StreamHandler) processGeminiSSELine(
	w http.ResponseWriter,
	flusher http.Flusher,
	eventType string,
	data []byte,
	state *geminiStreamState,
) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}

	var chunk types.GeminiStreamChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return fmt.Errorf("decode gemini stream: %w", err)
	}
	observeSourceModel(w, chunk.ModelVersion)
	if chunk.UsageMetadata != nil {
		state.observeUsage(chunk.UsageMetadata)
		if recorder, ok := w.(interface{ SetPartialUsage(*types.Usage) }); ok {
			recorder.SetPartialUsage(geminiUsageToAnthropic(state.usage))
		}
	}
	if err := upstreamJSONError("Gemini", chunk.Error); err != nil {
		return err
	}
	if eventType == "error" {
		return upstreamJSONError("Gemini", data)
	}
	if chunk.PromptFeedback != nil && chunk.PromptFeedback.BlockReason != "" {
		stop, err := geminiFinishReason(chunk.PromptFeedback.BlockReason)
		if err != nil {
			return err
		}
		state.stopReason = stop
	}

	if len(chunk.Candidates) > 0 {
		candidate := chunk.Candidates[0]
		for _, part := range candidate.Content.Parts {
			if err := validateGeminiOutputPart(part); err != nil {
				return err
			}
			if state.stopReason != "" && (part.Text != "" || part.FunctionCall != nil) {
				return fmt.Errorf("Gemini content arrived after finish reason")
			}
			if part.FunctionCall != nil {
				call, err := geminiToolCall(part.FunctionCall)
				if err != nil {
					return err
				}
				if err := state.closeBlock(w); err != nil {
					return ErrClientDisconnected
				}
				if err := writeSSEEvent(w, types.MessageEvent{
					Type: "content_block_start", Index: &state.contentIndex,
					ContentBlock: &types.ContentBlock{Type: "tool_use", ID: call.ID, Name: call.Name},
				}); err != nil {
					return ErrClientDisconnected
				}
				state.blockType = "tool_use"
				if err := writeSSEEvent(w, types.MessageEvent{
					Type: "content_block_delta", Index: &state.contentIndex,
					Delta: &types.Delta{Type: "input_json_delta", PartialJSON: call.Arguments},
				}); err != nil {
					return ErrClientDisconnected
				}
				if err := state.closeBlock(w); err != nil {
					return ErrClientDisconnected
				}
				state.hasTools = true
				flusher.Flush()
			}
			if part.Text != "" {
				blockType := "text"
				delta := types.Delta{Type: "text_delta", Text: part.Text}
				block := &types.ContentBlock{Type: blockType}
				if part.Thought {
					blockType = "thinking"
					block.Type = blockType
					delta = types.Delta{Type: "thinking_delta", Thinking: part.Text}
				}
				if state.blockType != blockType {
					if err := state.closeBlock(w); err != nil {
						return ErrClientDisconnected
					}
					startEvent := types.MessageEvent{
						Type:         "content_block_start",
						Index:        &state.contentIndex,
						ContentBlock: block,
					}
					if err := writeSSEEvent(w, startEvent); err != nil {
						return ErrClientDisconnected
					}
					state.blockType = blockType
				}

				event := types.MessageEvent{
					Type:  "content_block_delta",
					Index: &state.contentIndex,
					Delta: &delta,
				}
				if err := writeSSEEvent(w, event); err != nil {
					return ErrClientDisconnected
				}
				flusher.Flush()
			}
		}

		if candidate.FinishReason != "" && state.stopReason == "" {
			if err := state.closeBlock(w); err != nil {
				return ErrClientDisconnected
			}

			stop, err := geminiFinishReason(candidate.FinishReason)
			if err != nil {
				return err
			}
			state.stopReason = stop
			if state.hasTools && state.stopReason == "end_turn" {
				state.stopReason = "tool_use"
			}
			flusher.Flush()
		}
	}

	return nil
}
