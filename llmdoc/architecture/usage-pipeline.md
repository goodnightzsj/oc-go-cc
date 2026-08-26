# 用量管线（Usage Pipeline）

一条请求的 token 用量从上游到面板/下游的完整路径，以及各环节的语义约定（2026-08-26 修复后状态）。

## 链路

1. **入口**：`internal/handlers/messages.go` 接收 Anthropic `/v1/messages`，`CaptureOriginal` 记录原始请求（仅 debug_capture 开启时）。
2. **路由**：`internal/router/` 按场景选择模型（scenario / override / family override），产出模型链。
3. **发送**：`internal/provider/opencode_go.go` 的 `Stream`（真实流量主路径）或 `client/opencode.go` 的 `ChatCompletion`（registry 缺失兜底）；两者均在 debug_capture 开启时记录 upstream request/response（`CaptureBody` 异步 tee）。
4. **响应转换**：上游 OpenAI usage → Anthropic usage，`usageInfoToAnthropic`（`internal/transformer/stream.go:619`）→ `splitPromptTokens`：
   - OpenAI 标准：`prompt_tokens_details.cached_tokens` → cache_read；input = prompt − cached
   - DeepSeek 分区形：`prompt_cache_hit/miss_tokens` hit+miss == prompt → (miss, hit, 0)
   - 无缓存字段：全量当 input（最坏情形，成本会上浮）
5. **录制**：流量完成后 `history.RequestRecord`（含 CacheReadTokens/CacheCreationTokens）→ `internal/storage/requests.go` Insert → SQLite `requests` 表。
6. **成本**：`internal/storage/analytics.go` 的 `costForTokens` 按 seed_prices 估算（见 `reference/cache-billing-audit.md`）；provider 同步路径另有 `provider_usage` 表（平台真实账单快照，`cost_units`/1e8 = USD）。
7. **下游**：GUI（`internal/gui`）读 requests/analytics 汇总；本地 CompactGate 网关作为客户端读代理回传的 usage（cached_input_tokens 与代理 cache_read 同源）。

## 语义约定

- `requests.input_tokens`：修复前=全量 prompt；修复后=纯输入（不含缓存）。旧行无法回填，面板金额在 2026-08-26 11:19 UTC 后才准确。
- `requests.cache_read_tokens` / `cache_creation_tokens`：缓存读取/写入拆分；UI 用 `DisplayInputTokens()`（`internal/history/record.go`）汇总三大项。
- 平台 `cacheWrite5m/1h` 恒为 null → cache creation 按 input 价计。
- 两处时区注意：`requests.start_time` 为 +08:00 本地；OpenCode 账单为 UTC；CompactGate `time` 为 UTC。

## 已知缺口

- `ChatCompletionNonStreaming`（`internal/client/opencode.go:406`）没有任何 capture 调用：非流式请求的原始数据不被记录。
- 捕获记录含完整对话内容（敏感），仅应临时开启。