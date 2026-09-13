# MUST：记账与调试基线

以下约定是改动与排障时的硬约束（2026-08-26 审计后固定）。

## 缓存 token（不可回退）

- 上游缓存拆分有两种格式：OpenAI 标准 `prompt_tokens_details.cached_tokens` 与 DeepSeek 分区形 `prompt_cache_hit/miss_tokens`。任何 usage 解析/转换改动都必须保持两路兼容（`internal/transformer/stream.go:645` 的 `splitPromptTokens` 是唯一权威拆分点）。
- `requests.input_tokens` 必须是**纯输入**（不含缓存）；缓存单列 `cache_read_tokens` / `cache_creation_tokens`。UI 汇总用 `DisplayInputTokens()`，不要直接相加 input+output。

## 成本估算

- 价格**按平台分表**，一张表一个平台：`internal/storage/seed_prices_opencode_go.json` 与 `seed_prices_commandcode.json`。表的归属由 `site.Descriptor.RateTable` 声明，查价入口是 `PriceForProviderModel(provider, model)`，在该平台自己的表内做最长子串匹配。
- Hard: 定价必须带 provider。同一个模型名在不同平台价不同（`deepseek-v4-flash` 在 OpenCode Go 是 0.22/0.66，在 CommandCode 是 0.15/0.60），跨表命中会产生一个格式完全正确、数值错误的成本。没有发布费率的平台返回 unknown，不得回落到别家的表。
- Hard: 上游 usage 的 token 口径**按站点不同**。CommandCode 的 `tokensIn` 是毛值（含 cache read），本项目的 `input_tokens` 是净形；把毛值套进本项目的计价公式会双重扣减 cache。已实测：官方 19 条 run 用净形公式逐条复现（`price_commandcode_test.go`）。
- 平台账单的**粒度也不同**：`/alpha/usage/summary`（key 可用）覆盖账户**全部 mode**（含 CLI），按次明细 `/internal/usage` 只返回 `mode: api` 且**只认浏览器会话**。2026-09-13 实测 23 vs 21，差额 $0.02878 全在那 2 条非 API run 上——对账不按 mode 对齐就会把非代理流量误算成差额。
- 价格变更必须同步 `price_official_test.go`、`analytics_cost_test.go`、`platform_data_test.go` 的断言，并用真实账单反推验证（方法见 `reference/cache-billing-audit.md`；CommandCode 的回归见 `price_commandcode_test.go`，19 条官方 run 逐条复现）。
- 平台账单含 `costMultiplier`（lite 计划倍率），代理一律按基础价估算，不乘倍率。

## debug_capture

- 记录含完整对话内容，**仅调试期开启**。`logging.debug_capture`（`internal/config/config.go:236`）。
- `CaptureEntry.Data` 是 string（SSE 多文档流不能作为 json.RawMessage）。
- 流式上游捕获依赖 `CaptureBody`（`internal/client/opencode.go`）——关闭管道通过 Close 触发，任何替换必须保持该语义；`ChatCompletionNonStreaming` 仍无捕获（已知缺口）。

## 请求历史保留（不可回退）

- `storage.retention_days`：**负数禁用清理**；`0` 或省略仍使用默认 7 天；正整数为保留天数。语义在 `internal/storage/retention.go:17-20`（`days==0 → 7`）与 `:41-44`（负数记 `request retention cleanup disabled` 后直接返回）；默认值 `internal/storage/database.go:53`，overlay 仅在字段 `!= 0` 时生效（`database.go:78`）。
- 只清 `requests` 表（`retention.go:65`、`:71`），每小时一次（`:25`）；不触碰 `provider_usage`。
- Hard: 恢复历史**之前**必须先部署含负数禁用语义的版本；恢复后不得回滚到把非正数当 7 天的旧程序，否则下次启动会再次删除已恢复的行。方法与恢复顺序见 `docs/history-recovery.md`。

## 部署底线

- 远端部署 = `git pull origin main && bash scripts/prod-deploy.sh`；成功后以 `curl 127.0.0.1:3456/health` 验证。重启会短暂中断本地 AI（反代穿透）。
- 远端 `requests.start_time` 为 +08:00，OpenCode 账单与 CompactGate 为 UTC——跨端对比必须先换算。