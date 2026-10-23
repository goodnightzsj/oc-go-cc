# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build   # Build binary to bin/routatic-proxy (CGO disabled by default)
make run     # Run without building
make test    # Run tests with race detector
make lint    # go vet + test
make clean   # Remove build artifacts
make install # Build and install to $GOPATH/bin
make dist    # Cross-compile for all platforms

# Start proxy with dashboard (recommended)
./bin/routatic-proxy start

# Start proxy only (headless)
./bin/routatic-proxy serve
```

### Architecture

**routatic-proxy start** runs both the proxy server and GUI dashboard:
- Proxy listens on `127.0.0.1:3456` (configurable)
- Dashboard at `http://127.0.0.1:3445`
- Usage data persists to SQLite (`~/.local/share/routatic-proxy/data.db`) regardless of dashboard state
- Press Ctrl+C to stop both servers

**routatic-proxy serve** runs headless (no dashboard).

## Architecture

**Purpose:** routatic-proxy is a proxy server that sits between Claude Code and OpenCode Go. It intercepts Anthropic API requests, transforms them to OpenAI Chat Completions format, forwards them to OpenCode Go, and transforms responses back to Anthropic SSE.

**Model routing is config-driven, not code-driven.** All models are defined in `~/.config/routatic-proxy/config.json` — adding a new model requires no code changes. Go provider models are transformed to OpenAI Chat Completions format automatically. Zen models use endpoint classification via `ClassifyEndpoint()`. The router in `internal/router/` selects models by matching request content against scenario patterns defined in `scenarios.go`.

If a model's upstream doesn't support Anthropic tool format (`type: "custom"` server-tool shorthands), set `"anthropic_tools_disabled": true` in the model config to force it through the Chat Completions transform path instead of the raw Anthropic endpoint.

**Two API endpoints:**

- OpenAI endpoint (`/v1/chat/completions`) — used by most models (GLM, Kimi, MiMo, Qwen)
- Anthropic endpoint (`/v1/messages`) — used only by MiniMax models

**Available models:**

| Model | Provider | Type | Best For |
|-------|----------|------|----------|
| GLM-5.2 | Go | Premium | Complex reasoning, architecture decisions (new) |
| GLM-5.1 | Go | Standard | Complex patterns, tool operations |
| GLM-5 | Go | Standard | Reasoning tasks (deprecated May 14, 2026) |
| Kimi K3 | Go | Flagship | Latest Kimi, 1M context, 131K output, multimodal (new) |
| Kimi K2.7 Code | Go | Code specialist | Code generation, 32K output context |
| Kimi K2.6 | Go | Standard | General purpose, default fallback |
| Qwen3.7 Plus | Go | Fast | Streaming, low-latency (new) |
| Qwen3.7 Max | Go | Fast | Background tasks (new) |
| Qwen3.6 Plus | Go | Fast | Streaming fallback |
| Qwen3.5 Plus | Go | Fast | Simple read-only ops |
| MiniMax | Zen | Long context | 1M context window |
| MiMo | Go | Reasoning | Step-by-step reasoning |

`internal/client/opencode.go` routes Go provider models to Chat Completions; Zen models are classified by `models.ClassifyEndpoint()` in `internal/models/classifier.go`. If a model's upstream doesn't support Anthropic tool format, set `anthropic_tools_disabled: true` in config.

**Scenario detection priority** (`internal/router/scenarios.go`):

1. Long Context (>80K tokens, configurable) → MiniMax (1M context)
2. Complex (architectural patterns, tool operations) → GLM-5.2
3. Think (reasoning keywords in system prompt) → GLM-5.1
4. Background (simple read-only ops, no tools) → Qwen3.7 Max
5. Default → Kimi K2.6

**Model overrides:** two config blocks bypass scenario routing based on the requested model. `model_overrides` matches the `model` string **exactly** (best with CC-Switch, which sends a custom model string). `model_family_overrides` maps a Claude family keyword (`opus`, `sonnet`, `haiku`) via **case-insensitive substring** match, so the versioned IDs Claude Code sends natively (`claude-opus-4-20250514`) route without CC-Switch. Precedence: exact `model_overrides` → `model_family_overrides` (longest key first) → `respect_requested_model` → scenario routing. Both are wired through `ModelRouter.RouteWithOverride` / `RouteWithFamilyOverride` (`internal/router/model_router.go`) and merged with a deduplicated scenario safety-net chain in `buildModelChain` (`internal/handlers/messages.go`).

**Cost-based routing:** when `cost_routing.enabled` is set, `Selector` in `internal/router/selector.go` replaces the static primary model with automatic cheapest-model selection from the catalog. It applies `max_context_window` (hard cap on context window), `prefer_providers` (global provider filter, intersected with per-scenario preferences), and `penalty_per_provider` (per-provider cost penalty added during sort). Enabled via `cost_routing.enabled` or the legacy `enable_cost_based_routing` flag.

Per-scenario policy lives in `cost_routing.scenarios`, keyed by routing scenario name (`default`, `background`, `think`, `complex`, `long_context`, `fast`, `vision`, `vision_complex`, `vision_long_context` — the canonical list is `config.CostScenarioNames`, kept in sync with the `Scenario` constants by `TestCostScenarioNamesMatchRouterScenarios`). Each entry may set `requires_tools`, `requires_vision`, `requires_reasoning`, `min_context_window`, and `preferred_providers`. Every field is additive on top of what the request itself already implies — `requestConstraints` derives tool use, vision, reasoning and token count per request, so an unconfigured scenario simply picks the cheapest model that can serve the request. Unknown keys are rejected at config load time so a typo cannot silently do nothing. Scenarios are **not** part of the models.dev catalog; it only supplies `providers` and `models`.

**Catalog schema:** Models are keyed as `provider/model-name` (e.g., `opencode-go/glm-5.2`). The catalog (`~/.config/routatic-proxy/catalog/catalog.json`) contains:
- `providers` — Provider definitions with `name`, `base_url`, `enabled`
- `models` — Model definitions keyed by full key with fields:
  - `id` — Full key (matches the map key)
  - `name` — Display name
  - `limit.context` — Context window size
  - `rates.input`/`rates.output` — Cost per million tokens
  - `tool_call` — Whether tools are supported
  - `modalities.input`/`output` — Input/output types (`["text"]` or `["text", "image"]` for vision)
  - `reasoning` — Whether reasoning mode is supported

Resolution functions in `internal/catalog/resolve.go` extract the provider from the key prefix. `ResolvedModel.ModelID` is the model name only (without provider prefix); `ResolvedModel.CanonicalName` is the full key.

For streaming, the router downgrades to fast models (Qwen3.7 Plus) for better TTFT.

**Deprecated models:**
- GLM-5 — deprecated May 14, 2026; use GLM-5.1 or GLM-5.2

**Polymorphic field handling:** Anthropic's `system` and `content` fields accept both strings and arrays. `pkg/types/` uses `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats.

**Long-running stream policy:** The server-level `WriteTimeout` is 0. Each upstream attempt has a total timeout as well as a provider-specific idle budget renewed by incoming bytes; heartbeats do not extend the total timeout. ClinePass's model peek obeys both bounds and reports non-EOF read errors before returning a stream, including a read that returns a model frame and an error together. This lets the handler fall back before `message_start`; after downstream SSE payload has started, failures terminate that stream without switching models. Normal EOF preserves the peeked bytes. Client cancellation stops retries and is not an upstream failure signal. Regression coverage: `internal/handlers/cline_peek_fallback_test.go`, `internal/provider/cline_pass_peek_failure_test.go`.

**Provider-specific API keys:** Each provider (OpenCode Go, OpenCode Zen, AWS Bedrock, OpenRouter, CommandCode, ClinePass) has its own key pool. Provider-specific keys take precedence over global keys. This enables per-provider fallback strategies and key rotation.

Within one scope, `api_key` and `api_keys` are **one pool, not alternatives**: `EffectiveAPIKeys` (`internal/config/config.go`) returns the single key followed by the array, so `api_key` is used on its own when the array is empty and leads the pool when it is not. A key written in both fields appears once.

**The dashboard displays one effective key pool.** Startup migration and saves fold `api_key` into `api_keys` only when neither field has an active environment override; otherwise the original layout is preserved. `keyPoolFields` in `internal/config/config.go` owns the field and environment mappings. `recordKeySources` derives both `key_origins` and `key_indices` from the raw file after the same safe fold, preserving placeholder identity even when displayed keys are deduplicated. Editor patches use `key_pool_edit: true` to distinguish effective-pool edits from ordinary field-based imports. Config writes follow symlinks and use the shared `config.WriteFileAtomic` commit.

Environment variable overrides (single key):
- `ROUTATIC_PROXY_OPENCODE_GO_API_KEY`
- `ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY`
- `ROUTATIC_PROXY_AWS_BEDROCK_API_KEY`

Environment variable overrides (comma-separated keys for the pool):
- `ROUTATIC_PROXY_OPENCODE_GO_API_KEYS=key-1,key-2,key-3`
- `ROUTATIC_PROXY_OPENCODE_ZEN_API_KEYS=key-1,key-2`
- `ROUTATIC_PROXY_AWS_BEDROCK_API_KEYS=key-1,key-2`

Precedence: provider-specific pool → global pool. Within a scope the single key and the array pool together (single first); across scopes the provider's own credentials are used when it has any, and the global pool only when it has none. An environment variable sets the field it names and does not clear the other, so `*_API_KEY` adds to a file's `api_keys` rather than replacing it. A credential supplied only by the environment appears in the dashboard as a read-only row (`key_origins: "env"`): it is not in the file, so the form cannot write it.

## Key Files

- `cmd/routatic-proxy/main.go` — CLI entry point (cobra). Default config template is generated here.
- `internal/config/` — Config types and JSON loader with `${VAR}` env interpolation.
- `internal/transformer/` — Request/response format conversion (Anthropic ↔ OpenAI).
- `internal/router/fallback.go` — Circuit breaker per model (3 failures = 30s skip).
- `internal/handlers/models.go` — `GET /v1/models` (OpenAI-style listing). Used by provider-switching tools like CC-Switch's "Fetch Models" button; sources IDs from `ModelRouter.ListModels` (config aliases + `model_overrides` keys + catalog canonical names).
- `configs/config.example.json` — Reference config with all options documented.
- `internal/gui/` — Embedded HTTP server for the dashboard (serves static assets + API endpoints).
- `internal/gui/assets/` — HTML/CSS/JS for the dashboard (Overview, History, Analytics, Quota, Settings tabs).
- `internal/quota/` — OpenCode Go plan quota reader (5-hour rolling / weekly / monthly windows). Parsing rules ported from [ocusage](https://github.com/muzimu217/ocusage) (MIT).
- `internal/history/` — In-memory ring buffer (1000 entries, O(1) insert, thread-safe).
- `internal/metrics/` — In-process request counters (received, streamed, success, failed, model distribution).
- `internal/storage/` — SQLite persistence layer for request history, latency samples, and analytics. `requests.go` owns the history query and its summary cache; see "Request History (History tab)" below.

### Plan Quota (Quota tab)

`GET /api/quota` reports the OpenCode Go plan windows for every configured Go key. The usage endpoint is derived from `opencode_go.base_url` by swapping the API suffix for `/usage`; a base URL that does not look like an OpenCode API endpoint is rejected instead of falling back to `opencode.ai`, so a key meant for a private mirror is never sent to the public host. Keys are fetched in parallel and each one's result (or error) is reported separately; the response carries only a masked key hint, never the key. Answers are cached for 30 seconds because the endpoint is undocumented and rate-limited — the dashboard's own poll never reaches upstream more often than that.

The response also carries `model_limits`: the per-model monthly allowance table parsed from the Go docs pages (`internal/quota/limits.go`, `ParseModelLimits`), refreshed daily by `limitsLoop` (24h TTL, lazy first fetch via `ensureModelLimits`, zh page with en fallback). Pricing variants (Peak/Off-Peak, token-range splits) merge into one row per model. Since the upstream usage endpoint never reports per-model numbers, `model_usage` is built from this instance's own SQLite ledger (`monthlyModelUsage` — per-model stored cost within [monthly resets_at − 31d, resets_at), a 31-day subscription cycle, priced in the official Go price currency): each row's `used_usd` is calibrated against the official monthly total and then divided back by the model's pool multiplier (× 60/allowance), so row spend stays directly comparable to the docs allowance (a DeepSeek row reads ~$23 used of $30, not $46 — the ×2 pool conversion applies only to the table's total row, which reconstructs the pool-equivalent percent that reconciles with the endpoint's monthly percent, also shown on the account cards). The UI shows only models with ledger entries.

### Request History (History tab)

`GET /api/history` returns one server-side filtered page; `GET /api/history/summary` returns the matching aggregates and breakdowns. The dashboard fetches both in parallel and waits for the slower one, so the summary's cost is what the tab's load time is.

The summary uses one scan grouped by `model, provider, scenario, DATE(start_time)`, folded into each dimension in Go, replacing five separate scans. Results are cached on `Database` against the persistent `history_revision` counter. INSERT, UPDATE and DELETE triggers advance that counter in the writer's transaction, including writes by a separate costs-reconcile or usage-import process. Rollbacks preserve the previous revision; cache validation reads one counter row rather than scanning the request table.

Search matches `id`, `model`, `requested_model`, `provider`, `scenario` and `error_msg` case-insensitively, using `LIKE` without a `LOWER()` wrapper: `LIKE` is already case-insensitive for ASCII, and the wrapper roughly doubled the search time by defeating the comparison's own fast path. FTS5 was measured and rejected — this history is overwhelmingly one model, so the terms people actually search match nearly every row, where a `MATCH` scan costs more than the `LIKE` it would replace, at the cost of an index larger than the table.

`upstream_model` records the model the gateway actually ran (`provider_metadata.gateway.routing.canonicalSlug`), which is not the id we sent: ClinePass answers `cline-pass/deepseek-v4.1-flash-1017` with `vmc/deepseek-v4p1-flash-1017-contributor-fallbacks`. The history table's model column shows that value; the requested id is not repeated beneath it. `reasoning_effort` is the level the client asked for, validated without rewriting: a stated `output_config.effort` wins over a level this proxy derives from `thinking.budget_tokens`, because the first is the caller's words and the second is arithmetic. `thinking.type` ("enabled"/"disabled"/"adaptive") is a mode and is never stored as a level. Chat Completions and Responses output preserve `none` and `minimal`; only native Anthropic output raises them to `low`, in both the raw-body and DTO paths of `internal/provider/anthropic_payload.go`. That mapping does not mutate the original request or its recorded intent. **The column records intent, not effect**: ClinePass ignores the field, so a populated row is not evidence that the level was honoured — judge that from `reasoning_tokens`. The accepted levels live in one table (`core.NormalizeEffort`). `internal/provider/effort_boundary_test.go` checks all three output formats, both native paths, streaming/non-streaming requests and history intent.

`first_byte_ms` is time to first byte, stamped by the ClinePass peek when the first chunk arrives — NULL means unmeasured (a non-streaming request has no first byte), never zero, and it sorts last in both directions. Only ClinePass fills either column.

Responses name the served model too, for every caller downstream: streaming reads it from the first upstream chunk (the peek replays what it consumed, so the forward is byte-identical and TTFT is unchanged — `message_start` already had to wait for that byte), non-streaming uses `canonicalSlug`. This is what makes CompactGate's `response_model` truthful without changing CompactGate. Scenario is still recorded and filterable but no longer a column: this instance routes everything through `override` (the client names the model, so `DetectScenario` never runs), which made it a constant.

Every observation added to a stream body must be forwarded by every wrapper around it — `ctxReadCloser`, `captureReadCloser` and `peekedBody` each erased one at some point, five times in total, and `internal/client/upstream_channel_test.go` holds all of them across bare/capture/context/both. One of those was invisible to tests because the fixture ran with capture off while production runs with it on.

`channel` filters on `requests.upstream_channel`, the upstream channel the gateway reported serving the request on. Only ClinePass publishes one, so it narrows that platform's rows; every other platform stores NULL. `GET /api/history/channels?provider=` reports the channels this instance actually recorded, not the gateway's advertised candidate list — filtering by a channel that never served a request returns an empty page. The dashboard offers the control only while a concrete platform is selected, since "all platforms" is not a platform whose channel this could be.

### ClinePass channel pinning

The gateway accepts two pin shapes, one per pipeline, and the proxy writes both (`internal/provider/cline_pass.go`): `provider.only` for the direct pipeline (OpenRouter) and `providerOptions.gateway.only` for the planner (Vercel AI Gateway). `channel_pin` defaults to `deepseek` when unset.

**`only` and `has` are different mechanisms, measured 2026-10-23 against the live gateway with a fresh session per sample:**

| field | meaning | measured behaviour |
|---|---|---|
| `only` | name a channel | honoured on planner (16/16), ignored on direct (74/76 over 7 days) |
| `has` | require a capability | **hard refusal** — a capability the routed channel does not declare fails the request with `stream_initialization_failed` |

`has` is not a hint and not a preference: it does not move traffic, it rejects. The capability set is therefore **per model**, in `cline_pass.channel_pin_capabilities` keyed by model id:

```json
"channel_pin_capabilities": {
  "cline-pass/deepseek-v4.1-flash-1017": ["vision", "implicit-caching"],
  "cline-pass/deepseek-v4-pro":          ["implicit-caching"]
}
```

Measured acceptance, per model, `has` alone with no `only`:

| capability | v4.1-flash-1017 | v4-pro |
|---|---|---|
| `vision` | accepted | **refused** |
| `implicit-caching` | accepted | accepted |
| `reasoning` | accepted | accepted |
| `structured-output` | accepted | accepted |

Capabilities measured to be refused by every model that reaches the gateway: `tools`, `function-calling`, `streaming`, `json-mode`, `image-input`, `prompt-caching`. They are absent from the whitelist, so naming one fails config load rather than producing a 100%-failure model.

**`deepseek-v4.1-flash` (no `1017` suffix) is deliberately absent from the map.** Measured 2026-10-23 with key #0, six fresh sessions per case: all six returned `openai-compatible-private`, and it returned that same channel even for a request carrying `has: ["definitely-not-real-xyz"]` — which every model that reaches the gateway rejects outright with `stream_initialization_failed`. That model never reaches the pinning gateway, so a capability list for it would be written into the request and never read. Configuring it would look like protection while doing nothing.

Models confirmed to reach the pinning gateway, useful when testing `has` (all measured 2026-10-23): `deepseek/deepseek-v4.1-flash-1017` (vision accepted, served by `deepseek`), `deepseek/deepseek-v4-flash-vision-exp` (vision accepted, served by `deepinfra`), `deepseek/deepseek-v4-flash-0731` (vision refused), `deepseek-v4-pro` (vision refused). Those are the ones where `has` actually decides the request.

**The one-model list must never be shared.** A single global list containing `vision` fails every `deepseek-v4-pro` request. That measurement is also the explanation for the circulating claim that pinning "only works on 1017" — the cap set `["vision","implicit-caching"]` happens to be satisfied only by 1017, not because `has` is model-specific.

Unknown capability names are refused at config load (`config.ValidateChannelCapabilities`, whitelist `config.knownChannelCapabilities`), because the gateway's own refusal arrives on a live request after the pin is chosen and the key spent. Adding a capability to the whitelist requires measuring it first. A model absent from the map sends no `has` field at all (`omitempty`), which is the shape every earlier deployment already sent.

**Session stickiness confounds any probe of this.** The gateway pins a session to a channel: repeating a request inside one session returns the same channel regardless of the pin field, and only a fresh `X-Session-Id` per sample shows the real distribution. Three earlier rounds of testing this produced three contradictory conclusions for exactly that reason. When probing, pair each variant against a baseline **within the same session** and compare, or use a fresh session per sample; never compare runs that differ only in elapsed time.

Regression: `internal/config/channel_capability_test.go` (whitelist, per-model validation, empty config), `internal/provider/cline_pass_capability_test.go` (per-model injection, omission when unconfigured).

### Stream failure recording

A streaming attempt that fails **after `ssePayloadWritten`** cannot fall back — the SSE status line is already committed — so the client sees a truncated response. Four things about this are worth knowing:

- **The proxy never rotates a key on a mid-stream failure.** `keyPool.Settle` runs inside `baseProvider.doSelected` (`internal/provider/provider.go:77`), i.e. when the response *headers* arrive. A stream that opens 200 and then dies is already recorded as healthy, and `Release` cannot overwrite it (`selection.settle` returns early once `done` is set). A key whose upstream always answers 200 and then drops is therefore never cooled — the client just sees repeated truncation. This is a deliberate trade: settling at the header is what releases `inFlight` early, and a long stream would otherwise hold a concurrency slot for its whole duration.
- **A failure is recorded when the client could see it, not when it was billed.** `recordStreamFailure` used to return before `InsertRequest` whenever the upstream reported no usage, so the History tab and Analytics showed a 100% streaming success rate: measured 7 days, 17 streaming failures in the journal against **0** rows with `success = 0` across 13641 requests — and the failures that matter most are exactly those that died before producing anything to bill. The test is now `payloadWritten`, not `hasUsage`: once bytes are on the wire the caller received a truncated answer, while before that the caller was still waiting and chose to leave, which is not a failure of anything. Such a row is written with `details_known = 0` and no cost (`history.RequestRecord.UsageUnobserved`), so it counts as an outcome without entering any average or total. `UsageUnobserved` is a separate field rather than a use of `DetailsKnown` because that field's zero value means unknown while every ordinary row is a measured request — keying the column on it would mark every success unobserved.
- **Frequency, measured 2026-10-23:** of those 17, only **4** were client-visible truncations ("all upstream models failed after SSE payload started"); the other 13 were the client disconnecting (`stream read canceled by context`, `client disconnected`), which is not a fault. Three of the four were the total timeout on the retired `deepseek-v4.1-flash`, clustered around 03:00 with `no usage reported` — an upstream that stalled with nothing on the wire, not a proxy defect.
- **Timeout budget, set from measurement rather than a guess.** 30 days of streaming requests (40448 samples): 98.03% finish under 30s, p99 = 40.0s, p99.9 = 107.3s, p99.99 = 275.4s, max = 294.6s, and **nothing at or above 5 minutes**. Every request over three minutes succeeded. The proxy had been running with all three timeouts unset, so `ProviderTimeouts` fell through to the same 5-minute constant for both the inter-byte and the total budget — which collapsed two different questions into one number. They are now separate in `cline_pass`: `stream_timeout_ms = 150000` (2.5 min, 1.4x the p99.9 total) and `streaming_timeout_ms = 600000` (10 min, 2x the observed max), with `timeout_ms = 120000` unchanged for non-streaming. A stalled upstream now fails in 2.5 minutes instead of 5 while a genuine long generation has room. The residual risk is a first byte that takes longer than 150s on a very large prompt — under p99.9 of 107s, but not impossible.

### Analytics and Performance panels

Both tabs used to run several full-window `GROUP BY` queries per poll — five for Analytics, two more for Performance — each scanning the whole window. The scan is what costs on `modernc.org/sqlite`, so they now share one grouped pass per view and cache the result against the same `history_revision` counter the history summary uses.

`Analytics.Summary` groups by `model, provider, scenario, DATE(start_time)` once and folds the groups into the summary, the three breakdowns and the daily trend in Go. `Latency.Snapshot` does the same for the per-model percentile table and the outcome counts. Measured on a 32K-row window: 212ms → 78ms cold, 79µs warm; perf 115ms → 25ms cold, 16µs warm.

The hourly trend is a **separate, separately cached** query. Bucketing the same rows into an hour granularity costs about as much as the whole daily snapshot, and no panel asks for it by default; giving it its own cache key means opening an hourly view cannot evict the daily snapshot the rest of the tab is reading.

`Analytics.Window` and `Latency.Snapshot` snap to whole minutes. Unsnapped, the overview's rolling `?days=7` produced a new window — and therefore a new cache key — on every poll, so the most frequently requested aggregate was the only one that could never reuse an answer. Snapping is a property of the window rather than of the cache, so a hit and a fresh computation describe exactly the same rows. The "last minute" figure is the exception and keeps its real trailing-60s meaning.

Guards: `internal/storage/analytics_snapshot_test.go` (cross-dimension totals agree, each rate uses its own population, hourly and daily trends reconcile, the cache follows the table and separates windows), `analytics_throughput_test.go`, `analytics_baseline_test.go`.

### GUI Config Editing

The Settings tab exposes all config fields as editable form inputs. On save, only changed fields are sent to the backend as a JSON patch. The backend reads the current config from disk, merges the patch, writes back, and reloads atomically — the running proxy picks up changes immediately without restart.

**Partial update flow:**
1. Frontend builds a patch object with only fields the user changed (compared to the last loaded config)
2. Backend reads current config from disk via `config.LoadFromPath()`
3. Backend merges patch fields onto current config via JSON marshal/unmarshal
4. Backend validates essential fields (host, port)
5. Backend writes merged config to disk and calls `atomicCfg.Reload()`

### Fallback page: edits write through

The Fallback tab has no Save button and no Preview. The chain, its order, additions and removals, and the cross-platform switch are persisted at the moment they change.

Three constraints hold that together:

1. **Every mutation calls `persist()`.** Missing one draws a change the proxy never received. `TestFallbackPageHasNoSaveOrPreview` checks each mutating function.
2. **Writes are serialised.** Two quick edits would otherwise race: the later request returns 204 with a payload the earlier one had already superseded, leaving the page showing both edits while the config kept one. `pending` holds only the newest complete state and `persisting` keeps one request in flight.
3. **The cross-platform switch is sent only when it is what changed.** A chain edit therefore patches `fallbacks` alone and can never silently flip a routing policy.

The switch's handler pointed at `FallbackModule.onCrossPlatformChange()`, which did not exist, so toggling it threw — invisible until then because the old Save path read the checkbox directly rather than going through the handler.

### Dashboard view state

Navigation and preferences use different media. The active tab lives in the URL hash (`#history`) so deep links, reloads and the back button work; filters, date ranges, sort, page and the fallback page's platform/search live in `localStorage` under `routatic-proxy-view-state`. Filters were in the hash originally: a shared link carried the sender's filters, the address bar grew without bound, and browser hash normalisation dropped parameters so a reload reset them.

- `VIEW_CONTROLS` in `internal/gui/assets/app.js` is the **only** key table. Adding a filter is one row, not a branch; `buildViewState` and `applyViewState` both iterate it.
- The hash is written in exactly one place, `activateTab`, because tabs also switch programmatically (opening platform settings, jumping to history) and a click handler would miss those.
- `syncViewHash()` returns early when the hash already names the tab, so `hashchange` → `activateTab` → `syncViewHash` cannot loop.
- The fallback page's platform filter (`fplatform`) is separate from the global `platform` key: the first narrows the model picker, the second narrows history and analytics. Sharing one key would silently filter every other tab.

**Testing caveat:** `go:embed` snapshots assets into the test binary, so an injected-bug check against `assets/*.js` can silently pass through a stale copy. Verify front-end behaviour by running the built artifact (Node loading `app.js`) rather than trusting a Go test alone.

**Nil safety:** the two dependency-backed handlers fail explicitly rather than serving a second, divergent answer. `/api/metrics` falls back to the persisted roll-up when the in-process counters are empty, and reports zero counters when the metrics instance is absent. The history endpoints (`/api/history`, `/api/history/summary`, `/api/history/channels`) return **HTTP 503** without storage: the in-memory ring buffer is a live tail, not a filterable or paginated dataset, so there is no queryable history to serve.

### Adjacent deployment: workbuddy2api-panel

A separate service on the same host (`/workbuddy2api-panel`, `workbuddy2api.service`, `127.0.0.1:7863`, published at `https://wb.9962510.xyz/panel/`) turns WorkBuddy accounts into an OpenAI-compatible API. It is **not** part of this proxy and shares no code with it; it is listed here only because it lives on the same box and occupies one platform-shaped niche.

Local modifications, deliberately kept as a patch rather than a fork: `cmd/server/config.go`, `cmd/server/main.go` and `internal/upstream/transport.go` add a `resin` config block that installs `Transport.Proxy`. The account identity for Resin's sticky routing is taken from the outgoing `X-User-Id` header — **not** `Authorization`, which is an OAuth access token that rotates hourly and would move the account to a new egress IP on every refresh. Verified: one account holds one egress IP across repeats, two accounts hold different ones.

`/usr/local/bin/wb2api-update` (systemd timer, 6 hourly) pulls upstream, re-applies that patch, rebuilds, health-checks and rolls back on failure. It keeps the newest **3** version snapshots in `/root/wb2api-versions`. Two bugs were found and fixed while building it: `git checkout` without `--force` refuses to discard local changes and leaves the tree on the *old* commit with the patch still applied, so the next `git apply` reports "patch does not apply" for a conflict that does not exist; and `mapfile` is a bash builtin, so a `zsh -c` harness fails even though the script (which declares bash) is fine.

## Dual Release Channel System

This project uses a dual release channel system for separating beta and production releases:

### Beta Channel (Automatic)
- **Trigger:** Every push to `main` branch (see `.github/workflows/beta-release.yml`)
- **Version format:** `v{UPCOMING}-beta.{N}` (e.g., `v0.5.3-beta.1`), where `{N}` is a sequential counter
- **GitHub release:** Marked as `prerelease: true`
- **Docker tags:** `v{UPCOMING}-beta.{N}`, `beta-{UPCOMING}`, and `beta` (rolling pointer to newest beta)

Beta releases are fully automated and include:
- Test suite validation
- Cross-platform binary builds (darwin-amd64/arm64, linux-amd64/arm64, windows-amd64/arm64)
- macOS DMG with CGO-enabled binary
- AI-generated changelog from commits
- Docker images for linux/amd64 and linux/arm64

### Production Channel (Manual)
- **Trigger:** Manual `workflow_dispatch` on `releases` branch (see `.github/workflows/release.yml`)
- **Version format:** `vX.Y.Z` (semantic versioning)
- **GitHub release:** Marked as `prerelease: false` (stable)
- **Docker tags:** `vX.Y.Z`, `vX.Y`, `vX`, `latest`

Production releases include all beta features plus:
- Homebrew tap update (requires `HOMEBREW_PAT` secret)
- Scoop bucket update (requires `SCOOP_PAT` secret)

### Version Detection Script

`.github/scripts/get-versions.sh` is used by the beta workflow to:
1. Fetch tags from the `origin/releases` branch to get current production version (e.g., `v0.5.2`)
2. Increment the **patch** to the next version (e.g., `v0.5.3`) - **beta is based on the upcoming patch release**
3. Generate beta version by appending `-beta.{N}`, where `{N}` is `max(existing beta counters for this upcoming version) + 1` - **the counter resets to 1 once the upcoming version ships as stable**
4. Output both versions as JSON for CI consumption


**Version Format Explanation:**
- `v0.5.3` = The upcoming production version (patch incremented from latest production)
- `beta.1` = Sequential prerelease counter for that upcoming version
- Full example: stable `v0.5.2` → `v0.5.3-beta.1`, then `v0.5.3-beta.2`, ... until `v0.5.3` ships → `v0.5.4-beta.1`

### Creating a Production Release

1. Merge all changes to `main` and verify via beta
2. Ensure `releases` branch exists and is up-to-date
3. Go to GitHub Actions → Release workflow
4. Click "Run workflow"
5. Enter version (must follow `vX.Y.Z` format)
6. Workflow validates, builds, and releases

### Release Workflow Stages

Both workflows share the same stages:

1. **validate** — Run `go vet`, `go test -race`, and build sanity check on ubuntu-latest
2. **release** — Build cross-platform binaries and macOS DMG on macos-latest
3. **docker** — Publish multi-arch Docker images on ubuntu-latest

Production adds:
4. **homebrew** — Update the homebrew-tap formula
5. **scoop** — Update the scoop-bucket manifest

## Skill routing

When the user's request matches an available skill, invoke it via the Skill tool. When in doubt, invoke the skill.

Key routing rules:
- Product ideas/brainstorming → invoke /office-hours
- Strategy/scope → invoke /plan-ceo-review
- Architecture → invoke /plan-eng-review
- Design system/plan review → invoke /design-consultation or /plan-design-review
- Full review pipeline → invoke /autoplan
- Bugs/errors → invoke /investigate
- QA/testing site behavior → invoke /qa or /qa-only
- Code review/diff check → invoke /review
- Visual polish → invoke /design-review
- Ship/deploy/PR → invoke /ship or /land-and-deploy
- Save progress → invoke /context-save
- Resume context → invoke /context-restore
- Author a backlog-ready spec/issue → invoke /spec
