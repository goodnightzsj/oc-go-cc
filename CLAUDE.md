# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build   # Build binary to bin/oc-go-cc
make run     # Run without building
make test    # Run tests with race detector
make lint    # go vet + test
make clean   # Remove build artifacts
make install # Build and install to $GOPATH/bin
make dist    # Cross-compile for all platforms
```

Run a single test: `go test ./internal/router/ -v`

## Architecture

**Purpose:** oc-go-cc is a proxy server that sits between Claude Code and OpenCode Go. It intercepts Anthropic API requests, transforms them to OpenAI Chat Completions format, forwards them to OpenCode Go, and transforms responses back to Anthropic SSE.

**Model routing is config-driven, not code-driven.** Models are defined in `~/.config/oc-go-cc/config.json` — adding a new model does not require code changes (except for `IsAnthropicModel()` if the new model uses the Anthropic endpoint). The router in `internal/router/` selects models by matching request content against scenario patterns defined in `scenarios.go`.

**Two API endpoints:**

- OpenAI endpoint (`/v1/chat/completions`) — used by most models (GLM, Kimi, MiMo, Qwen)
- Anthropic endpoint (`/v1/messages`) — used only by MiniMax models

`internal/client/opencode.go` routes by model ID via `IsAnthropicModel()`.

**Scenario detection priority** (`internal/router/scenarios.go`):

1. Long Context (>80K tokens, configurable) → MiniMax (1M context)
2. Complex (architectural patterns, large-scale design) → GLM-5.1
3. Think (reasoning keywords in system prompt) → GLM-5
4. Background (simple read-only ops, no tools) → Qwen3.5 Plus
5. Default → Kimi K2.6

Note: `hasComplexPattern` is intentionally narrow — common coding/debugging words
("build", "debug", "bash") do NOT route to Complex (upstream `889a758`).

For streaming, the router downgrades to fast models (Qwen3.6 Plus) for better TTFT.

**Capacity filtering** (`internal/router/capacity.go`, `internal/config/model_registry.go`):
- A model chain is filtered by declared capacity only when some model has
  `context_window` / `max_output_tokens` set (or resolves to one via the
  built-in model registry). Otherwise the chain passes through unchanged
  (zero regression).
- `config.ResolveModelConfig` fills default ContextWindow/MaxOutputTokens/Vision
  from `modelMetadata` (`internal/config/model_registry.go`) so known models get
  capacity limits without per-model config.
- A small client `max_tokens` (e.g. Claude Code's 64-token safety classifier)
  does NOT cause a skip — only an exhausted context window does (upstream `0ef0705`).
- Capacities are re-checked per request in `buildModelChain`
  (`internal/handlers/messages.go`).

**SSE idle watchguard** (`internal/transformer/idle.go`, `stream.go`):
- `ProxyStream` runs a `StartIdleWatchdog`; if no upstream bytes arrive within
  `client.RequestTimeout(model)` (default 5 min), the stuck stream is released
  and reported as `ErrStreamIdle`, triggering the fallback chain instead of
  holding the connection forever (upstream `3b7d987`).
- DeepSeek mid-conversation `system` messages are rewritten to
  `<system-reminder>` user messages so DeepSeek's system-reordering does not
  invalidate the prefix KV cache (upstream `e3ba31b`).
- Single-key deployments short-circuit the fallback chain on upstream auth
  errors (401/403): a bad key fails every model identically, so trying each
  fallback only burns attempts and opens every breaker. Multi-key deployments
  skip the short-circuit so key rotation can still succeed
  (`internal/server/server.go` wires the predicate via
  `FallbackHandler.SetAuthSingleKey`; upstream `f5d7e79`).

**Polymorphic field handling:** Anthropic's `system` and `content` fields accept both strings and arrays. `pkg/types/` uses `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats.

## Key Files

- `cmd/oc-go-cc/main.go` — CLI entry point (cobra). Default config template is generated here.
- `internal/config/` — Config types and JSON loader with `${VAR}` env interpolation.
- `internal/transformer/` — Request/response format conversion (Anthropic ↔ OpenAI).
- `internal/router/fallback.go` — Circuit breaker per model (3 failures = 30s skip).
- `configs/config.example.json` — Reference config with all options documented.
