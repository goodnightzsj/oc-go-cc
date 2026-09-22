# MUST：记账与调试基线

以下约定是改动与排障时的硬约束（2026-08-26 审计后固定）。

## 缓存 token（不可回退）

- 上游缓存拆分有两种格式：OpenAI 标准 `prompt_tokens_details.cached_tokens` 与 DeepSeek 分区形 `prompt_cache_hit/miss_tokens`。任何 usage 解析/转换改动都必须保持两路兼容（`internal/transformer/stream.go:645` 的 `splitPromptTokens` 是唯一权威拆分点）。
- `requests.input_tokens` 必须是**纯输入**（不含缓存）；缓存单列 `cache_read_tokens` / `cache_creation_tokens`。UI 汇总用 `DisplayInputTokens()`，不要直接相加 input+output。

## 成本估算

- 价格**按平台分表**，一张表一个平台（`seed_prices_opencode_go.json`、`seed_prices_commandcode.json`、`seed_prices_cline_pass.json`，映射见 `internal/storage/database.go:615-619` 的 `rateTableFiles`）。表的归属由 `site.Descriptor.RateTable` 声明，在该平台自己的表内做最长子串匹配。
- 查价签名是 `PriceForProviderModel(provider, model string, inputTokens int64)`（`internal/storage/database.go:651`），`inputTokens` 用于选长上下文价格档（传 0 取基础档）。它**不是成本的唯一入口**：逐请求成本走 `costForProviderTokensAt`（`internal/storage/pricing.go:31`），后者按 prompt 档位选价并叠乘峰谷倍率后再调 `costForTokens`（`internal/storage/analytics.go:242`）；逐行录入成本另走 `requests.costForRecord`（`internal/storage/requests.go:486`）。
- Hard: 定价必须带 provider。同一个模型名在不同平台价不同，跨表命中会产生一个格式完全正确、数值错误的成本。两表确实分离的守卫是 `TestPriceTablesArePerPlatform`（`internal/storage/price_test.go:29`），它用 `claude-opus-4-8` 断言两平台价格不等——**不要用 `deepseek-v4-flash` 举例**，该行在 OpenCode Go 与 CommandCode 现同为 0.15/0.60，已不再能证明差异。没有发布费率的平台返回 unknown，不得回落到别家的表。
- Hard: 上游 usage 的 token 口径**按站点不同**。CommandCode 的 `tokensIn` 是毛值（含 cache read），本项目的 `input_tokens` 是净形；把毛值套进本项目的计价公式会双重扣减 cache。已实测：官方 19 条 run 用净形公式逐条复现（`price_commandcode_test.go`，对账行从 `:52` 起，`TestCommandCodeRunsMatchThePlatformBill` 在 `:42`）。
- 平台账单的**粒度也不同**：`/alpha/usage/summary`（key 可用）覆盖账户**全部 mode**（含 CLI），按次明细 `/internal/usage` 只返回 `mode: api` 且**只认浏览器会话**。2026-09-13 实测 23 vs 21，差额 $0.02878 全在那 2 条非 API run 上——对账不按 mode 对齐就会把非代理流量误算成差额。
- 价格变更必须同步 `price_official_test.go`、`analytics_cost_test.go`、`platform_data_test.go` 的断言，并用真实账单反推验证（方法见 `reference/cache-billing-audit.md`；CommandCode 的回归见 `price_commandcode_test.go`，19 条官方 run 逐条复现）。
- 平台账单含 `costMultiplier`（lite 计划倍率），代理一律按基础价估算，不乘倍率。

## 峰谷与分档

- Hard: **估算错时要错在"少收"一侧**。规则读不准时应回落基础价，而不是猜一个高价档。两处实际取舍都按这个方向定：条件读不懂的 `pricing.overrides` 一律**跳过**（条件未读的 override 会退化成无条件，按列表恰好排第一的档定价），未覆盖的时间窗模型回落底价。反过来猜会向用户报一个虚高的账单。
- Hard: **倍率对齐计价来源的底价，不是上游载荷的 headline 价**。两处数据源对同一个模型可能记不同的档：OpenRouter 的 `tencent/hy3` 载荷 lead 0.132/0.528、折扣 0.0825/0.33，而 models.dev（本项目实际计价来源）记的底价是后者，所以倍率是 1.6 不是 2。`TestOpenRouterPeakMatchesThePublishedOverrides`（`internal/history/peak_test.go:292`）钉住这个算术；**若 models.dev 改用 headline 底价，这些倍率会双重计算，此测试是暴露点**。
- 分档阈值的比较对象是**完整 prompt**（新鲜输入 + 缓存读 + 缓存写，`promptTokensOf`，`internal/storage/requests.go:539`），不是仅新鲜输入。只比新鲜输入会把缓存重的请求放进便宜档，而上游按贵档计费。阈值是**严格大于**。
- 规则形态是**每 rule 自带** `models`/`windows`/`multiplier`/`allDays`（`internal/history/record.go:153-176`），不是每平台一条——同一个平台的模型可以在窗口方向、倍率、适用星期上各不相同。`allDays` 零值安全：漏写保持工作日限制，失败方向同样是少收。

## debug_capture

- 记录含完整对话内容，**仅调试期开启**。`logging.debug_capture`（`internal/config/config.go:324`）。
- `CaptureEntry.Data` 是 string（SSE 多文档流不能作为 json.RawMessage）。
- 流式上游捕获依赖 `CaptureBody`（`internal/client/opencode.go`）——关闭管道通过 Close 触发，任何替换必须保持该语义；`ChatCompletionNonStreaming` 仍无捕获（已知缺口）。

## 请求历史保留（不可回退）

- `storage.retention_days`：**负数禁用清理**；`0` 或省略仍使用默认 7 天；正整数为保留天数。语义在 `internal/storage/retention.go:17-20`（`days==0 → 7`）与 `:41-44`（负数记 `request retention cleanup disabled` 后直接返回）；默认值 `internal/storage/database.go:53`，overlay 仅在字段 `!= 0` 时生效（`database.go:80`）。
- 只清 `requests` 表（`retention.go:65`、`:71`），每小时一次（`:25`）；不触碰 `provider_usage`。
- Hard: 恢复历史**之前**必须先部署含负数禁用语义的版本；恢复后不得回滚到把非正数当 7 天的旧程序，否则下次启动会再次删除已恢复的行。方法与恢复顺序见 `docs/history-recovery.md`。

## 部署底线

- 远端部署 = `git pull origin main && bash scripts/prod-deploy.sh`；成功后以 `curl 127.0.0.1:3456/health` 验证。重启会短暂中断本地 AI（反代穿透）。
- `requests.start_time` **新行写 UTC**（`internal/storage/requests.go:54` 的 `rec.StartTime.UTC().Format(time.RFC3339Nano)`）；**历史行可能带 +08:00**，`parseRequestTime`（`internal/storage/pricing.go:12`）两者都接受。OpenCode 账单与 CompactGate 为 UTC——与历史行对比前必须确认该行是哪种后缀，否则日期错位 8 小时。