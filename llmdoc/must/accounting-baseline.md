# MUST：记账与调试基线

以下约定是改动与排障时的硬约束（2026-08-26 审计后固定）。

## 缓存 token（不可回退）

- 上游缓存拆分有两种格式：OpenAI 标准 `prompt_tokens_details.cached_tokens` 与 DeepSeek 分区形 `prompt_cache_hit/miss_tokens`。任何 usage 解析/转换改动都必须保持两路兼容（`internal/transformer/stream.go:645` 的 `splitPromptTokens` 是唯一权威拆分点）。
- `requests.input_tokens` 必须是**纯输入**（不含缓存）；缓存单列 `cache_read_tokens` / `cache_creation_tokens`。UI 汇总用 `DisplayInputTokens()`，不要直接相加 input+output。

## 成本估算

- 价格唯一来源 `internal/storage/seed_prices.json`，最长子串匹配（`PriceForModel`）。价格变更必须同步 `price_official_test.go` 与 `analytics_cost_test.go` 的断言，并用真实账单反推验证（方法见 `reference/cache-billing-audit.md`）。
- 平台账单含 `costMultiplier`（lite 计划倍率），代理一律按基础价估算，不乘倍率。

## debug_capture

- 记录含完整对话内容，**仅调试期开启**。`logging.debug_capture`（`internal/config/config.go:236`）。
- `CaptureEntry.Data` 是 string（SSE 多文档流不能作为 json.RawMessage）。
- 流式上游捕获依赖 `CaptureBody`（`internal/client/opencode.go`）——关闭管道通过 Close 触发，任何替换必须保持该语义；`ChatCompletionNonStreaming` 仍无捕获（已知缺口）。

## 部署底线

- 远端部署 = `git pull origin main && bash scripts/prod-deploy.sh`；成功后以 `curl 127.0.0.1:3456/health` 验证。重启会短暂中断本地 AI（反代穿透）。
- 远端 `requests.start_time` 为 +08:00，OpenCode 账单与 CompactGate 为 UTC——跨端对比必须先换算。