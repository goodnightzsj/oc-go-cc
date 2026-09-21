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

Here `provider` selects one of the same five providers; omission retains the legacy **OpenCode Go** default. `refresh=1` bypasses the 30-second Go/OpenRouter/CommandCode cache. AWS GET requests only read the cached billing snapshot, even with `refresh=1`; they never initiate paid queries. Responses carry `provider`, `status`, `source`, `reason` when applicable, `fetched_at`, `ttl_seconds`, `cached`, official `links`, and separate account results. They are sent with `Cache-Control: no-store`.

| Status | Meaning |
| --- | --- |
| `available` | The configured upstream account query succeeded |
| `partial` | Some account/key queries succeeded and others failed |
| `not_configured` | Required credentials are missing or AWS billing is disabled |
| `unavailable` | No public account API, no matching billing data, or an explicit AWS query is required; inspect `reason` |
| `error` | The configured query failed; error details are preserved without credentials |

Go `accounts[].report` contains the existing quota windows. OpenRouter `accounts[].openrouter` contains the individual inference key's cap, remaining cap, UTC usage, and BYOK usage; absent optional values remain `null`. Key caps are **not** an account balance, and multiple keys are not summed. `credits`, `credits_status`, and `credits_error` are separate, using only `openrouter.management_api_key` (or `ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY`). A negative `total_credits - total_usage` balance is retained.

OpenRouter quota queries require explicit `openrouter.api_key` / `api_keys` (or their provider-specific environment overrides). Merely viewing this platform never probes its endpoint with the legacy global key pool. Go quota retains its legacy global-key compatibility; inference routing's existing key precedence is unchanged.

CommandCode returns `source=official_alpha_api`, `currency=USD`, and one `accounts[].commandcode` report per distinct configured key. Each report has `credits`, `subscription`, and `usage`, with independent `credits_error`, `subscription_error`, and `usage_error` for partial failures. If all three requests fail, the account has `error` instead. A successful null subscription remains absent without an error. Credits are USD-denominated usage credits, not cash; monthly grant is unknown and no monthly percentage is inferred. Window `resetAt` values are Unix milliseconds, with zero meaning unknown. Only provider-specific keys are used; no browser cookie, global key fallback, identity or payment fields. See [account fields and scope](commandcode.md#commandcode-账户查询).

Zen returns `reason=no_public_account_api`. Bedrock returns `aws_billing_disabled` until explicitly enabled, `aws_billing_refresh_required` without a current snapshot, or `aws_billing_no_data` when AWS returns no matching costs. All five providers' local usage remains independently available through the analytics endpoint. See the [capability matrix](platform-integration-review.md#五平台页面与账户能力).

CommandCode `accounts[].ledger`, when present, contains this instance's `requests`, `known_requests` (known request outcomes, not price coverage), `unknown_cost_requests`, and the known `cost_usd` subtotal for the account's subscription period. Unknown prices must not be displayed as a definite zero. The ledger is omitted when multiple keys prevent per-account attribution or no valid billing period is available.

### `POST /api/quota?provider=aws-bedrock&billing_refresh=1`

Explicitly queries the official Cost Explorer API. This action may incur AWS API charges; it requires `aws_bedrock.billing.enabled=true`, a validated 12-digit `linked_account_id`, and the service's standard AWS SDK credentials or named `profile`. Inference keys are never used. Cross-origin browser POSTs are rejected with HTTP 403; the paid query via GET, POST without the action flag, or POST for another provider returns HTTP 405.

`bedrock_billing` contains `linked_account_id`, `start_date`, exclusive `end_date`, `metric=UnblendedCost`, exact included `services`, reported `currency`, nullable `total_cost`, `estimated`, and `daily` rows (`date`, `cost`, `estimated`). The query covers 30 complete UTC days and services whose AWS billing names contain Bedrock. This is not account credit, an invoice, or the local proxy ledger; see [service coverage and setup](aws-bedrock-billing.md). Negative and zero costs are preserved; missing data stays unknown. Any failed page or invalid amount/currency discards the partial report and returns an explicit error state.

The snapshot has `ttl_seconds=86400`, is invalidated by billing profile/account changes or the UTC date changing, and is kept only in memory. Ordinary reads never refresh it upstream. Another explicit POST requests a new bill; overlapping POSTs reuse a completed in-flight result when they share the same scope.

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
| 502 | All upstream models failed, with no more specific cause retained |
| 503 | The active platform (`active_site`) has no routing target for this request |

The proxy forwards several platforms, so a refusal the platform named is
reported as that refusal: its status code and message are passed through
unchanged (`401`, `403 MODEL_NOT_IN_PLAN`, `404`, `422`, `429` from upstream),
truncated at 4 KB. This matters operationally because an intermediary may
replace the body of a 5xx — Cloudflare substitutes its own error page for a
`502`, which previously hid the platform's reason entirely.

Content the target wire format cannot carry is **dropped, not rejected**: only
the representable parts of a `tool_result` reach the upstream, and a result with
no representable part becomes an empty tool message. The request still succeeds.
This covers Claude Code's `tool_reference` (ToolSearch) and an image inside a
`tool_result`; see [CommandCode 已知限制](commandcode.md) for the trade this
makes.

### 524 from Cloudflare (2026-09-14)

A `524` is the **edge** giving up, not the proxy returning an error. Cloudflare's
Proxy Read Timeout is **120 s** and is not raisable below Enterprise; nginx
behind it allows `client_body_timeout 300s` and `proxy_read_timeout 600s`, so
the edge is the binding constraint. Cloudflare closes the connection and the
client sees `origin_response_timeout`, while nginx logs a **`499`** (client
closed) with an empty body.

**Read `urt` first — it separates two different causes.** `urt = -` means nginx
never opened a connection upstream, so the time went into receiving the body; a
populated `urt` means the request reached the proxy and the proxy (or its
upstream) was slow. Both surface as the same `524` and the same `499`, and the
fixes are opposite:

| Mode | nginx evidence | Where the 120 s went | Fix |
| --- | --- | --- | --- |
| Body-bound | `urt=- uct=- uht=-` | Receiving the request body | Smaller body, or a faster client link |
| Upstream-bound | `urt` populated, `uht=-` | Proxy held the connection without sending headers | Investigate the proxy/upstream latency |

Observed 2026-09-14, three `499`s out of 5196 requests, all at ~125 s. Only the
first is confirmed to be the reported `524` — its `cf_ray` matches the client's
error verbatim — but the other two show the same `499`-at-125 s shape in the
other mode:

```
12:21:27 claude-cli  rt=125.008 urt=-      uct=-     uht=-      ← body-bound (the reported 524)
01:42:42 curl       rt=125.018 urt=91.197  uct=0.001 uht=-      ← upstream-bound
01:43:56 curl       rt=125.015 urt=91.919  uct=0.001 uht=-      ← upstream-bound
```

For the body-bound case, `urt=-` is conclusive that this is **not** a proxy
fault. Its own body size is unknowable from the capture — a request that never
reached the proxy is never captured — but the surrounding session shows the
scale: adjacent requests carried **5.04–5.06 MB**, of which ~5.17 MB is the
`messages` array (1027 messages) and only ~93 KB is `tools`. The `524`'s start
time (12:19:21, from `rt`) falls exactly in the gap between the previous
completed request (12:18:14) and the next one (12:22:00), consistent with the
same session's growing context. `client_max_body_size` is not the limit — it is
100m, orders of magnitude above what is being sent.

The upstream-bound pair is a separate, unresolved signal: `urt` of ~91 s means
the proxy held the connection that long before headers (`uht=-`), and RFC-style
diagnosis would look at the proxy's own routing/transform latency rather than at
body size. It is not this incident, and no cause is asserted for it here.

The reported error carries `retryable: true`, so a client retry resolves it.
Above Enterprise, the only structural fix for the body-bound case is sending
less per request; removing the orange cloud would expose the origin IP.

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
