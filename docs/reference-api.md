# HTTP API Reference

routatic-proxy exposes Anthropic Messages and a supported stateless subset of OpenAI Responses. Both use the same routing, rate limiting, fallback, and accounting pipeline. The proxy listens on loopback by default; it does not authenticate incoming client tokens. Do not expose it publicly without a separately configured authentication boundary.

## Endpoints

### `POST /v1/messages`

The primary endpoint. Accepts Anthropic Messages API requests and returns responses in the same format.

**Request body** — standard Anthropic `MessageRequest`:

```json
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 4096,
  "system": "You are a helpful assistant.",
  "messages": [
    {
      "role": "user",
      "content": "Hello, world!"
    }
  ],
  "stream": true,
  "tools": []
}
```

**Response** — Anthropic `MessageResponse` (non-streaming) or SSE stream (streaming).

**Routing behavior:**

- If `model` matches an entry in `model_overrides` (exact match), that model is used as primary with a scenario-derived safety net
- Otherwise, if `model` contains a family keyword configured in `model_family_overrides` (`opus`/`sonnet`/`haiku`, case-insensitive substring), that mapped model is used as primary with a scenario-derived safety net
- Otherwise, scenario-based routing selects the model based on request content and token count
- `respect_requested_model: false` disables direct requested-model resolution, but explicit exact/family overrides still take precedence

**Headers:**

| Header | Value |
|--------|-------|
| `X-Request-ID` | Unique request identifier (generated or forwarded from client) |
| `Content-Type` | `application/json` (non-streaming) or `text/event-stream` (streaming) |

### `POST /v1/responses`

Accepts the stateless Responses subset used by the tested Codex configuration. Example:

```json
{
  "model": "commandcode",
  "input": "Hello, world!",
  "stream": true,
  "store": false
}
```

Supported inputs include text, user images, system/developer instructions, JSON function calls, and string function results. Function declarations require `strict: false`. Both streaming and non-streaming responses include usage; cached input is included in Responses `input_tokens` and separately identified by `input_tokens_details.cached_tokens`. Local storage continues to keep fresh input and cache tokens in separate columns.

Unsupported options fail with HTTP 400 rather than being silently discarded: server-side state (`store: true`, `previous_response_id`, `conversation`), background work, nonempty `include`, reasoning summaries/encrypted reasoning, hosted or custom/freeform tools, strict/structured output, and unsupported multimodal parts. There are no response retrieval/deletion or WebSocket endpoints. See [Codex setup](commandcode.md#codex) for the tested client settings and exact boundaries.

Streaming emits Responses events such as `response.created`, `response.output_text.delta`, `response.function_call_arguments.delta`, and a terminal response event. An incomplete or invalid upstream stream produces a failure rather than a fabricated successful completion. Errors before streaming use an OpenAI-style `error` object with `type`, `message`, `param`, and `code`.

`anthropic_first` only wraps `/v1/messages`; `/v1/responses` always uses configured model providers.

### `POST /v1/messages/count_tokens`

Counts tokens for a message array without generating a response.

**Request body:**

```json
{
  "system": "System prompt text",
  "messages": [
    { "role": "user", "content": "Hello" }
  ]
}
```

**Response:**

```json
{
  "input_tokens": 42
}
```

### `GET /v1/models`

Returns the set of model identifiers a client may request, in the OpenAI
`/v1/models` envelope. Two consumers use it:

- **[CC-Switch](../CONFIGURATION.md#using-with-cc-switch)** — its "Fetch Models"
  button populates a model dropdown from this endpoint.
- **Claude Code gateway model discovery** — when enabled, Claude Code calls
  `GET /v1/models?limit=1000` and adds the results to its `/model` picker
  (labeled "From gateway"). It reads the `display_name` field and **only
  surfaces models whose `id` begins with `claude` or `anthropic`** — all other
  ids are silently filtered from the picker (see
  [Claude Code model picker](../CONFIGURATION.md#claude-code-model-picker)).

The listing merges config `models` aliases, `model_overrides` keys, and — when
a catalog is available — catalog canonical names (`provider/model`). Any value
in the list is valid in the `model` field of `POST /v1/messages`.

**Response:**

```json
{
  "object": "list",
  "data": [
    { "id": "default", "object": "model", "owned_by": "opencode-go" },
    { "id": "claude-sonnet-4-5-20250929", "object": "model", "owned_by": "opencode-zen" },
    { "id": "opencode-go/kimi-k2.6", "object": "model", "owned_by": "opencode-go", "name": "Kimi K2.6", "display_name": "Kimi K2.6" }
  ]
}
```

The `limit` query parameter (sent by Claude Code) is accepted and currently
ignored — the full list is always returned. Only `GET` is allowed; other
methods return `405`.

### `GET /health`

Returns server health status.

**Response:**

```json
{
  "status": "ok",
  "version": "1.2.3",
  "models_configured": 6,
  "uptime": "2h30m"
}
```

### `GET /statusline`

Returns compact status for TUI integration (statusline, tmux bar).

**Response:**

```json
{
  "status": "running",
  "version": "1.2.3",
  "uptime": "2h30m"
}
```

## Dashboard APIs

These endpoints are served by `start` on the dashboard listener (default `127.0.0.1:3445`), not the inference port. Keep this listener private; configuration writes are privileged operations.

### Provider-scoped queries

`provider` accepts `opencode-go`, `opencode-zen`, `aws-bedrock`, `openrouter`, or `commandcode`. Legacy underscore spellings normalize to hyphens. Omit the parameter (or use an empty value) for all providers; an unsupported value returns HTTP 400 rather than silently widening the query.

| Endpoint | Other query parameters | Scope |
| --- | --- | --- |
| `GET /api/history` | Existing search, date, status, model, sort, `page` and `size` filters | Paged records from the selected provider |
| `GET /api/history/summary` | Same record filters | Matching summary and breakdowns |
| `GET /api/analytics/summary` | `days`, or RFC3339 `from` + `to`; `compare=1` with `days` | Summary, model/provider/scenario breakdowns; comparison includes scoped `today`, `retained`, and `last_minute` |
| `GET /api/analytics/tokens/trend` | Same time range; `granularity=day\|hour` | UTC time buckets for the selected provider |
| `GET /api/perf/models` | `range` | Provider + model latency and known outcomes |
| `GET /api/perf/aggregate` | `range` | Selected-provider latency and known success/failure totals |

Explicit analytics ranges are `[from, to)` and limited to 92 days. The configured storage baseline still applies. `/api/metrics` describes the whole running process, not the selected provider. Without persistent storage, provider-specific performance returns HTTP 503 instead of all-provider in-memory data.

Unknown cost is not free usage: `unknown_cost_requests` accompanies the known monetary subtotal. Requests without known outcome details do not contribute to a success-rate denominator. Identical model IDs on different providers remain separate rows.

Empty analytics collections (`models`, `providers`, `scenarios`, and `trend`) and the `/api/perf/models` response are JSON arrays (`[]`), not `null`, including newly configured platforms with no local requests.

### `GET /api/quota`

Here `provider` selects one of the same five providers; omission retains the legacy **OpenCode Go** default. `refresh=1` bypasses the 30-second per-provider cache. Responses carry `provider`, `status`, `source`, `reason` when applicable, `fetched_at`, `ttl_seconds`, `cached`, official `links`, and separate account results. They are sent with `Cache-Control: no-store`.

| Status | Meaning |
| --- | --- |
| `available` | The configured upstream account query succeeded |
| `partial` | Some account/key queries succeeded and others failed |
| `not_configured` | Required credentials are missing |
| `unavailable` | This provider's account capability is not integrated; inspect `reason` |
| `error` | The configured query failed; error details are preserved without credentials |

Go `accounts[].report` contains the existing quota windows. OpenRouter `accounts[].openrouter` contains the individual inference key's cap, remaining cap, UTC usage, and BYOK usage; absent optional values remain `null`. Key caps are **not** an account balance, and multiple keys are not summed. `credits`, `credits_status`, and `credits_error` are separate, using only `openrouter.management_api_key` (or `ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY`). A negative `total_credits - total_usage` balance is retained.

OpenRouter quota queries require explicit `openrouter.api_key` / `api_keys` (or their provider-specific environment overrides). Merely viewing this platform never probes its endpoint with the legacy global key pool. Go quota retains its legacy global-key compatibility; inference routing's existing key precedence is unchanged.

Zen and CommandCode return `reason=no_public_account_api`. Bedrock returns `reason=aws_billing_auth_required`; no Cost Explorer integration or credential fallback is implied. All five providers' local usage remains independently available through the analytics endpoint. See the [capability matrix](platform-integration-review.md#五平台页面与账户能力).

### Configuration writes

`GET /api/proxy/config` and `GET /api/config/export` redact credentials. `POST /api/proxy/config` accepts a partial JSON object; provider settings merge with existing siblings, while routing maps and arrays replace their previous values. Validation failure preserves both the file and live configuration. Redacted placeholders do not overwrite existing secrets; an empty management key removes the file-level setting (an environment override still takes precedence).

## Error Responses

Messages application errors follow Anthropic's error format (method/rate-limit rejections may be plain text). Responses errors use the envelope described above:

```json
{
  "type": "error",
  "error": {
    "type": "api_error",
    "message": "description of what went wrong"
  }
}
```

**HTTP status codes:**

| Code | Meaning |
|------|---------|
| 400 | Invalid request body |
| 405 | Method not allowed (non-POST on /v1/messages) |
| 413 | Request body too large (>100MB) |
| 429 | Rate limited |
| 500 | Internal error (routing failed, transform error) |
| 502 | All upstream models failed |

## Streaming

Streaming responses use Server-Sent Events (SSE) with Anthropic's event format:

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_...","type":"message","role":"assistant","content":[],"model":"...","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":42,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":15}}

event: message_stop
data: {"type":"message_stop"}
```

**Heartbeat**: keepalive comments (`:keepalive\n\n`) are sent every 3 seconds during streaming.

## Rate Limiting

The proxy applies per-IP rate limiting (default: 100 requests/minute). Rate-limited requests receive HTTP 429.

## State and Duplicate Requests

The proxy does not expose a `request_dedup` configuration option or server-side Responses conversation storage. Each admitted request is processed independently; clients must resend the supported conversation input they need.
