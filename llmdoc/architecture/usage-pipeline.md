# 用量管线（Usage Pipeline）

一条请求的 token 用量从上游到面板/下游的完整路径，以及各环节的语义约定（2026-08-26 修复后状态）。

## 链路

1. **入口**：`internal/handlers/messages.go` 接收 Anthropic `/v1/messages`，`CaptureOriginal` 记录原始请求（仅 debug_capture 开启时）。Responses 入站最终汇入同一条管线，见 `reference/inbound-protocols.md`。
2. **路由**：`internal/router/` 按场景选择模型（scenario / override / family override），产出模型链。
3. **发送**：统一由core.Provider的Execute/Stream发送，OpenRouter也在registry内，无legacy HTTP兜底。捕获路径复用client.CaptureBody异步tee，Close等待捕获回调。各平台协议与捕获覆盖见 `architecture/provider-layer.md`。
4. **响应转换**：上游 OpenAI usage → Anthropic usage，`usageInfoToAnthropic`（`internal/transformer/stream.go:629`）→ `splitPromptTokens`：
   - OpenAI 标准：`prompt_tokens_details.cached_tokens` → cache_read；input = prompt − cached
   - DeepSeek 分区形：`prompt_cache_hit/miss_tokens` hit+miss == prompt → (miss, hit, 0)
   - 无缓存字段：全量当 input（最坏情形，成本会上浮）
5. **录制**：流量完成后 `history.RequestRecord`（含 CacheReadTokens/CacheCreationTokens）→ `internal/storage/requests.go` Insert → SQLite `requests` 表。 每次执行独立生成UUID，外部X-Request-ID只关联回显/日志/capture，重复关联ID不覆盖记账；中断流保留已报告用量与失败状态。
6. **成本**：逐请求成本入口是 `costForProviderTokensAt`（`internal/storage/pricing.go:31`），它在 `costForTokens`（`internal/storage/analytics.go:242`）之上叠乘 `history.ProviderPeakMultiplier(provider, model, t)`（`pricing.go:40`/`:51`），并按 **prompt 档位**选价——`PriceForProviderModel(provider, model, in+cacheRead+cacheCreate)` 的三参数形态（`pricing.go:38`）。长上下文分档来自 `models.cost_tiers` 列（建列 `internal/storage/database.go:335`，由 `internal/catalog/types.go` 填值），应用点在 `internal/storage/requests.go:507-519`，比较对象是完整 prompt（`promptTokensOf`，`requests.go:539`）。provider 同步路径另有 `provider_usage` 表（平台真实账单快照，`cost_units`/1e8 = USD）。
   平台费率表不再是纯构建期快照：`internal/storage/pricerefresh.go` 每小时从平台自己的页面刷新（`DefaultPriceRefreshInterval = time.Hour`，`:56`），seed 文件是刷新失败时的回退。另见 `reference/cache-billing-audit.md`。
7. **下游**：GUI（`internal/gui`）读 requests/analytics 汇总；本地 CompactGate 网关作为客户端读代理回传的 usage（cached_input_tokens 与代理 cache_read 同源）。

## 语义约定

- `requests.input_tokens`：修复前=全量 prompt；修复后=纯输入（不含缓存）。旧行无法回填，面板金额在 2026-08-26 11:19 UTC 后才准确。
- `requests.cache_read_tokens` / `cache_creation_tokens`：缓存读取/写入拆分；UI 用 `DisplayInputTokens()`（`internal/history/record.go`）汇总三大项。
- 平台 `cacheWrite5m/1h` 恒为 null → cache creation 按 input 价计。
- 两处时区注意：`requests.start_time` 新写入行已是 UTC（`internal/storage/requests.go:54` 的 `rec.StartTime.UTC().Format(time.RFC3339Nano)`）；历史行可能带 +08:00，`internal/storage/pricing.go:12` 的解析器两者都接受。OpenCode 账单为 UTC；CompactGate `time` 为 UTC。

## 面板时间筛选

各页并非同一个时间窗口，不应仅凭“最近 7 天”文案直接对账。日期范围在各自时区包含结束日，API 上界为次日零点（左闭右开）；只查询本实例仍保留、且符合对应统计口径的记录。

| 页面 | 默认与口径 | URL 状态 |
|------|------------|----------|
| 概览 | 默认最近 7 天，可选 7/30/90；从当前时刻回退 N×24 小时，非 N 个日历日；“今天”卡片单独按 UTC | 范围仅保存在当前页面内存，重载恢复 7 天 |
| 历史请求 | 默认不限日期；日期输入与今天/7/30 天快捷按浏览器本地时区，快捷立即应用 | `from` / `to` |
| 用量分析 | 默认含当天的 7 个 UTC 日历日；7/30/90 天快捷需应用；最多 92 天 | `afrom` / `ato`；小时/天粒度不写入 URL |
| 性能 | 默认全部，可选最近 1h/24h/7d，按经过时长回退 | `range` |
| 用量与账单的本地账本 | 默认最近 30 天，可选 7/30/90；与概览同为相对 N 天 | `days` |

官方配额/账单独立于本地账本：按上游窗口展示；AWS 手动查询最近 30 个完整 UTC 日，不含当天。设置、降级策略没有时间筛选。来源：`internal/storage/analytics.go:52`、`internal/storage/latency.go:221`、`internal/gui/assets/app.js:1730`。

历史和分析弹窗均支持“结束日期始终为今天”：开始日期固定，隐藏结束值及 URL 分别保存 `to=today` / `ato=today`；发请求时由 `resolvedEndDate` 按本地/UTC 解析为实际边界，不把标记传给后端。动态范围始终显式保存开始日期，即使它恰好等于初始化默认值，重载也不能滑动开始日期。页面可见时复用 3 秒轮询，跨日及重载自动推进结束日期；分析的固定范围仍按原方式手动刷新。见 `internal/gui/assets/app.js:1807`、`:2199`、`:5032`。

checkbox 和日期输入是弹窗草稿，应用才生效；取消/Escape 后重开恢复已应用值。快捷日期退出动态模式，清除历史范围也清除动态状态。非法日期和倒置范围显式报错；动态分析跨日超过 92 天时提示调整开始日期，不截断范围或绕过后端限制。回归：`internal/gui/date_range_behavior_test.go:5`。

## 已知缺口

- OpenRouter Execute/Stream均捕获request/response，旧ChatCompletionNonStreaming已删除；其它provider的捕获覆盖以各自实现为准。
- 捕获记录含完整对话内容（敏感），仅应临时开启。
