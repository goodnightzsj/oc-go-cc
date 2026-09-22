# OpenRouter Provider

[OpenRouter](https://openrouter.ai) is a unified API for 200+ LLMs from OpenAI, Anthropic, Google, Meta, Mistral, and other leading AI providers. It provides a single endpoint for accessing models from multiple vendors without managing separate API keys and integrations for each provider.

## Overview

### What is OpenRouter?

OpenRouter acts as a universal gateway to the AI model ecosystem. Instead of maintaining separate accounts and API keys for OpenAI, Anthropic, Google, and dozens of other providers, you use a single OpenRouter API key to access them all. OpenRouter handles the routing, normalization, and billing.

### Benefits

- **Unified API**: One endpoint, one authentication method for 200+ models
- **Automatic failover**: If a provider is down, requests can route to alternatives
- **Standardized pricing**: Clear per-token costs across all providers
- **Model exploration**: Easily experiment with new models without new integrations
- **OpenAI-compatible format**: Works with existing OpenAI SDKs and tools
- **No code changes**: Add new models via configuration only

## Getting Started

### 1. Sign Up and Get API Key

1. Sign up at [openrouter.ai](https://openrouter.ai)
2. Generate an API key at [https://openrouter.ai/keys](https://openrouter.ai/keys)
3. Add funds to your account (pay-as-you-go pricing)

### 2. Configure Environment Variables

Set the environment variable:

```bash
export ROUTATIC_PROXY_OPENROUTER_API_KEY=sk-or-v1-your-key-here
```

For key rotation or load balancing across multiple keys, use a comma-separated list:

```bash
export ROUTATIC_PROXY_OPENROUTER_API_KEYS=key-1,key-2,key-3
```

### 3. Enable in Config

Add the `openrouter` provider to your `~/.config/routatic-proxy/config.json`:

```json
{
  "openrouter": {
    "api_key": "${ROUTATIC_PROXY_OPENROUTER_API_KEY}",
    "base_url": "https://openrouter.ai/api/v1/chat/completions"
  }
}
```

This is a configuration fragment to merge into an existing file. The runtime reads the top-level `openrouter` object, not `providers.openrouter`. Configure a model target with `provider: "openrouter"`; adding credentials alone does not change the routing rules.

## Configuration

### Configuration Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `base_url` | `string` | No | Complete Chat Completions endpoint. Default: `https://openrouter.ai/api/v1/chat/completions` |
| `api_key` | `string` | Yes* | Single API key for authentication. Required if `api_keys` not set |
| `api_keys` | `string[]` | Yes* | Multiple API keys for round-robin rotation. Required if `api_key` not set |
| `management_api_key` | `string` | No | Separate management key for account Credits only; never used for inference |
| `timeout_ms` | `int` | No | Request timeout in milliseconds. Default: `300000` (5 minutes) |
| `stream_timeout_ms` | `int` | No | Per-chunk timeout during streaming. Default: `60000` (1 minute) |
| `streaming_timeout_ms` | `int` | No | Total streaming-attempt timeout; when unset uses `timeout_ms` |

*Provider-specific inference keys take precedence over the legacy global key pool. Prefer an explicit OpenRouter key so a global key intended for another provider is not reused. A management key alone does not authorize inference.

### Environment Variables

| Variable | Description | Precedence |
|----------|-------------|------------|
| `ROUTATIC_PROXY_OPENROUTER_API_KEY` | Single API key override | Highest |
| `ROUTATIC_PROXY_OPENROUTER_API_KEYS` | Comma-separated keys for round-robin | Highest |
| `ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY` | Account Credits management key | Highest |

Environment variables take precedence over config file values. Config values support `${VAR}` interpolation. There is no automatic `ROUTATIC_PROXY_OPENROUTER_BASE_URL` override; use `openrouter.base_url` directly or reference your own environment variable there.

Precedence order: `*_API_KEYS` → `*_API_KEY` → config file `api_keys` → config file `api_key`

## Configuration Examples

### Single-Key Setup

```json
{
  "openrouter": {
    "api_key": "${ROUTATIC_PROXY_OPENROUTER_API_KEY}"
  }
}
```

### Multi-Key Round-Robin

For load balancing across multiple API keys:

```json
{
  "openrouter": {
    "api_keys": ["${OPENROUTER_KEY_1}", "${OPENROUTER_KEY_2}"]
  }
}
```

### Custom Base URL

For enterprise/self-hosted OpenRouter deployments:

```json
{
  "openrouter": {
    "base_url": "https://openrouter.mycompany.com/api/v1/chat/completions",
    "api_key": "${OPENROUTER_API_KEY}"
  }
}
```

### Complete Configuration with Models

```json
{
  "host": "127.0.0.1",
  "port": 3456,
  "respect_requested_model": false,
  "catalog": {"enabled": false},
  "openrouter": {
    "api_key": "${ROUTATIC_PROXY_OPENROUTER_API_KEY}",
    "base_url": "https://openrouter.ai/api/v1/chat/completions",
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000
  },
  "models": {
    "default": {
      "provider": "openrouter",
      "model_id": "openai/gpt-4o",
      "max_tokens": 4096
    }
  },
  "model_overrides": {
    "openrouter": {
      "provider": "openrouter",
      "model_id": "openai/gpt-4o",
      "max_tokens": 4096
    }
  }
}
```

The model above is an example, not a claim of current account access. Replace `model_id` with an available OpenRouter model, run `routatic-proxy validate`, and request the `openrouter` alias from your client.

## Quota and Account Credits

Select **OpenRouter** in the Quota tab. Local usage includes only requests that pass through this instance; it is fetched separately from upstream account data.

- `GET /api/v1/key` uses only explicitly configured OpenRouter inference keys (`openrouter.api_key` / `api_keys` or their provider-specific environment overrides) and returns that key's cap and UTC daily/weekly/monthly usage. Unlike legacy inference routing, browsing quota never falls back to global keys. BYOK usage is shown separately. A `null` cap means no per-key cap, not an unlimited account balance; missing optional usage values remain unknown.
- `GET /api/v1/credits` requires a separate **Management Key**. Set `openrouter.management_api_key` in Settings, or `ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY` in the service environment. It is never put in the inference pool and never falls back to the inference key. Account balance is `total_credits - total_usage`, including negative balances.
- Multiple keys are not summed or assumed to belong to one account. Errors are shown per key; one failed key does not hide another key's data. The response is cached per platform for 30 seconds and invalidated by endpoint/key changes.
- Custom gateways must implement these endpoints at their own configured origin. The proxy never sends a private gateway key to the public OpenRouter host as a quota fallback.

Official contracts: [Key limits and UTC usage](https://openrouter.ai/docs/api_reference/limits), [account Credits and Management Key requirement](https://openrouter.ai/docs/api/api-reference/credits/get-remaining-credits). Local HTTP response fields are documented in the [dashboard API reference](reference-api.md#dashboard-apis).

## Pricing Source and Time-Window Rates

### Prices come from the catalog

OpenRouter publishes no per-token table this proxy parses, so `site.RateTable` is
empty for it and its rates are read from the models.dev catalog instead, matched
against the request's model id. That path was silently broken until 2026-09-22:
the catalog's `Model` carried its rates under the JSON tag `rates` while
models.dev publishes them under `cost`, so every catalog model parsed with no
rates and every OpenRouter request was booked as unknown. Three other platforms
hid it by having hand-written seed tables; OpenRouter is the platform priced only
from the catalog. The mapping is now explicit and tested (see
`internal/catalog/types.go`).

### Prefixed ids are not a lookup problem

An OpenRouter model id carries a vendor prefix (`openai/gpt-5.6-terra-pro`), and
the catalog stores it that way. Lookup is `provider = ? AND name = ?`, with the
same prefixed form on both sides, so the prefix is preserved rather than
stripped - verified against a real catalog import. Stripping it would in fact
introduce a defect: 83 of OpenRouter's variant ids (`:batch`, `:free`) carry
prices that differ from their base model's, so folding them onto the base name
would bill them at the wrong rate.

### Time-window pricing is not modelled

OpenRouter does express peak/off-peak, through per-model `pricing.overrides` on
its own `/api/v1/models` endpoint. As of 2026-09-22 exactly two models use the
time-window form:

| Model | Window | Effect |
|-------|--------|--------|
| `deepseek/deepseek-v4.1-flash` | weekday `utc_start`/`utc_end` 0100-0400 and 0600-1000, plus all weekend | x2 on the listed base rate |
| `tencent/hy3` | 0000-1600 UTC discounted, 1600-0000 full | **inverted** relative to DeepSeek: peak is 16:00-24:00 UTC |

Two things follow, and neither is implemented here. `history.peakSchedules`
models a *platform-wide* window with one multiplier, and `hy3` shows neither
holds: the multiplier differs per model (x1.6 there, x2 elsewhere) and the
window can point the other way. And models.dev - the source this proxy
actually reads - does not carry `overrides` at all, so nothing has been lost by
omitting it: the data is simply not fetched. A model with time-window pricing is
therefore billed at its listed base rate, which for both of the above is the
*cheaper* band, so the error is under-billing rather than over-billing.

The 67 other models with overrides use `min_prompt_tokens` long-context tiers
rather than time windows. Those are also not modelled by this path, and they move
money in the other direction: a >200K-token request on `x-ai/grok-4.7` costs
double what the catalog's single rate says.

Both gaps are recorded rather than approximated. Fixing them means teaching the
catalog import to read `overrides` and the pricing path to apply tiers and
per-model windows, which is a change to the shared cost path rather than to
OpenRouter alone.

## Cost-Based Routing Integration

OpenRouter works seamlessly with `cost_routing`. Use `penalty_per_provider` to adjust effective costs:

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["openrouter", "opencode-go"],
    "max_context_window": 1000000,
    "penalty_per_provider": {
      "openrouter": 0.02,
      "opencode-go": 0.0,
      "aws-bedrock": 0.05
    }
  }
}
```

Penalties are additive to the raw model cost. Example: a model costing $0.10/1M tokens on OpenRouter with a 0.02 penalty has effective cost $0.12/1M tokens. Use this to bias routing preferences without excluding providers entirely.

### Applying Cost Penalty to OpenRouter

When using `cost_routing`, you can apply a penalty to OpenRouter requests to account for routing overhead or prefer direct providers when costs are similar:

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["opencode-go", "openrouter"],
    "penalty_per_provider": {
      "openrouter": 0.05
    }
  }
}
```

This adds a small cost penalty (e.g., 5 cents per million tokens) when selecting OpenRouter models, helping the router prefer direct providers when cost is comparable.

## Model Selection and Naming Convention

### Provider/Model Format

OpenRouter uses the `provider/model-name` format. Models are referenced using the `openrouter/` prefix followed by the provider and model name:

```
openrouter/{provider}/{model-name}
```

Examples:
- `openrouter/openai/gpt-4o`
- `openrouter/anthropic/claude-3.5-sonnet`
- `openrouter/google/gemini-2.0-flash-exp`
- `openrouter/meta-llama/llama-3.3-70b-instruct`

### Model Resolution via Catalog

Models are referenced using the `provider/model-name` pattern. OpenRouter models use the `openrouter/` prefix:

```json
{
  "model_overrides": {
    "claude-opus-4": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-opus-4",
      "temperature": 0.7,
      "max_tokens": 8192,
      "vision": true
    },
    "gpt-4o": {
      "provider": "openrouter",
      "model_id": "openai/gpt-4o",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "gemini-2.5-pro": {
      "provider": "openrouter",
      "model_id": "google/gemini-2.5-pro-preview-07-11",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  }
}
```

The `model_id` in your config must match OpenRouter's model identifier exactly.

### Discovering Models

1. Visit [openrouter.ai/models](https://openrouter.ai/models) for the complete model list
2. Use the `routatic-proxy models` command to see cached catalog entries
3. Check the [OpenRouter API docs](https://openrouter.ai/docs) for pricing and context limits

## Model Examples

| Model Key | Provider | Description | Best For |
|-----------|----------|-------------|----------|
| `openai/gpt-4o` | OpenAI | Latest GPT-4o multimodal model | General purpose, vision tasks |
| `openai/o1` | OpenAI | Reasoning model (o1) | Complex reasoning, math, coding |
| `openai/gpt-4.5-preview` | OpenAI | GPT-4.5 preview | Advanced reasoning, research |
| `anthropic/claude-3.5-sonnet` | Anthropic | Claude 3.5 Sonnet | Coding, analysis, writing |
| `anthropic/claude-3-opus` | Anthropic | Claude 3 Opus | Most capable Anthropic model |
| `anthropic/claude-3.5-haiku` | Anthropic | Claude 3.5 Haiku | Fast, cost-effective tasks |
| `anthropic/claude-opus-4` | Anthropic | Claude Opus 4 | Deep reasoning, coding |
| `google/gemini-2.0-flash-exp` | Google | Gemini 2.0 Flash (experimental) | Low latency, high throughput |
| `google/gemini-pro-1.5` | Google | Gemini 1.5 Pro | Long context (up to 2M tokens) |
| `google/gemini-2.5-pro-preview-07-11` | Google | Gemini 2.5 Pro | Advanced multimodal tasks |
| `meta-llama/llama-3.3-70b-instruct` | Meta | Llama 3.3 70B | Open source, self-hostable |
| `meta-llama/llama-3.1-405b` | Meta | Llama 3.1 405B | Largest open source model |
| `mistralai/mistral-large` | Mistral | Mistral Large | Strong multilingual performance |
| `mistralai/mistral-medium` | Mistral | Mistral Medium | Balanced performance/cost |
| `mistralai/mistral-small` | Mistral | Mistral Small | Fast, efficient tasks |
| `deepseek/deepseek-chat` | DeepSeek | DeepSeek V3 | Strong reasoning, coding |
| `deepseek/deepseek-r1` | DeepSeek | DeepSeek R1 | Reasoning, step-by-step |
| `perplexity/sonar-reasoning` | Perplexity | Sonar Reasoning | Research, citations |

See the full catalog at [https://openrouter.ai/models](https://openrouter.ai/models).

## Use Cases

### Accessing Specific Models

Use OpenRouter when you need models not available on other providers:

```json
{
  "models": {
    "complex": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-opus-4",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max"
    }
  }
}
```

### Fallback Chains

Include OpenRouter as a fallback when primary providers fail:

```json
{
  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "deepseek-v4-pro" },
      { "provider": "openrouter", "model_id": "anthropic/claude-sonnet-4.8" },
      { "provider": "openrouter", "model_id": "openai/gpt-4.1" }
    ]
  }
}
```

### Cost Optimization

Use `cost_routing` with provider penalties to automatically select the cheapest available model:

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["openrouter"],
    "penalty_per_provider": {
      "openrouter": -0.01
    }
  }
}
```

## Official Documentation

- **API Reference**: [https://openrouter.ai/docs](https://openrouter.ai/docs)
- **OpenAI Compatibility**: [https://openrouter.ai/docs#openai-compatibility](https://openrouter.ai/docs#openai-compatibility)
- **Provider Routing**: [https://openrouter.ai/docs#provider-routing](https://openrouter.ai/docs#provider-routing)
- **Models Catalog**: [https://openrouter.ai/models](https://openrouter.ai/models)

## Catalog Resolution Details

Resolution functions in `internal/catalog/resolve.go` extract the provider from the key prefix. For OpenRouter models:

- `ResolvedModel.ModelID` is the model name only (without provider prefix)
- `ResolvedModel.CanonicalName` is the full key (e.g., `openrouter/anthropic/claude-opus-4`)

The catalog schema for models includes:

| Field | Description |
|-------|-------------|
| `id` | Full key (matches the map key) |
| `name` | Display name |
| `limit.context` | Context window size |
| `rates.input` | Cost per million input tokens |
| `rates.output` | Cost per million output tokens |
| `tool_call` | Whether tools are supported |
| `modalities.input` | Input types (`["text"]`, `["text", "image"]`) |
| `modalities.output` | Output types (`["text"]`, `["text", "image"]`) |
| `reasoning` | Whether reasoning mode is supported |

For streaming, the router may downgrade to faster models for better TTFT (time to first token).

---

**Note**: OpenRouter models use the OpenAI Chat Completions API format. The proxy automatically handles request/response transformation between Anthropic and OpenAI formats.
