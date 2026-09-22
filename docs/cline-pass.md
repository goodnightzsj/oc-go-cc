# ClinePass 接入

核实日期：2026-09-21。接入使用官方 API key，不读取或修改用户的全局客户端配置，不触碰 Cline 的订阅账号体系。

## 为什么这个平台接得进来

ClinePass 是 Cline 的 $9.99/月订阅（[文档页](https://docs.cline.bot/getting-started/clinepass)），提供 13 个开源模型、约 2–5× 标准 API 费率的用量。官方文档有一个专节 *"Using ClinePass outside of Cline"*，原文：

> You can use ClinePass models from your own scripts, apps, or automation through the Cline API.

也就是说第三方调用是**官方支持的路径**，不是逆向。本项目只做个人自用代理：用户自己的 key、正常速率。

| 项 | 值 | 来源 |
| --- | --- | --- |
| 推理端点 | `POST https://api.cline.bot/api/v1/chat/completions` | [Chat Completions](https://docs.cline.bot/api/chat-completions) |
| 协议 | OpenAI Chat Completions（**无** Anthropic 端点） | [API 概览](https://docs.cline.bot/api/overview) |
| 鉴权 | `Authorization: Bearer <key>` | [Authentication](https://docs.cline.bot/api/authentication) |
| 模型 ID | `cline-pass/<name>` | 同文档页 |
| 用量窗口 | 5 小时滚动 / 日历周 / 日历月 | ClinePass 页 `## Usage` |
| 配额端点 | `GET /api/v1/users/me/plan/usage-limits` | 见「配额」节 |

**不采用的路线**：`luawei1/cline2api`、`pingmike2/cline2api-workers` 一类项目走 WorkOS 设备授权换取账号 refreshToken、再跑多账号轮询。那条路同时命中 Cline ToS §2.2(4)（key 交易）、§7.3(c)(v)（禁止他人使用你的订阅）与 §2.2(1)（逆向），而 §1.3 又给 Cline 无理由终止权、§7.1 声明不退费。本项目不实现，也不为它留接口。

## 平台身份：ID 是 `cline-pass`，不是 `cline`

`site.Descriptor.ID` 会写进 `requests.provider`，是存储契约。这里选 `cline-pass` 而非 `cline` 不是命名偏好，是因为**代码用模型 ID 的前缀当 provider 键**：

| 机制 | 位置 | 语义 |
| --- | --- | --- |
| `ProviderFromModelKey` | `internal/catalog/types.go:94` | 取**第一个 `/` 之前**作为 provider |
| `modelSupportsProvider` | `internal/router/selector.go:195` | **精确相等**比较 |
| `Resolve` 的 provider 校验 | `internal/catalog/resolve.go:66` | 不等即报 `model %q is not available on provider %q` |

而 Cline 的模型 ID 前缀**就是计费池标识**：

| 池 | 前缀 | 计费 |
| --- | --- | --- |
| ClinePass | `cline-pass/` | $9.99/月订阅 |
| 免费促销 | **混用** `cline-free/`、`z-ai/`、`poolside/` | 免费额度 |
| 云 | `cline-cloud/` | 云计费 |
| 用量计费 | 厂商前缀（`anthropic/`、`openai/`…） | 按量付费 |

免费池前缀不统一，所以**判断计费池要看桶（feed），不能看前缀**——Cline 自家 SDK 的注释也是这样写的（`catalog-cline-recommended.ts:111`：*"The feed bucket determines free access, regardless of the ID namespace."*）。

由此，若 site ID 取 `cline`：

- models.dev 的 `cline-pass` provider（**已存在**，15 个模型）经 `Sync` 全量落盘后，catalog 里出现 `cline-pass/glm-5.3` 这类键，`ProviderFromModelKey` 切出 `"cline-pass"`，与 site ID `cline` 不等；
- 于是凭证绑定 `ProviderAPIKeys("cline-pass")` 落空、成本路由的候选被过滤掉、`Resolve` 直接报错。

`cline` 这个词描述的东西（"Cline 平台"）在代码里**不对应任何前缀**，等于人为制造一条跨路径不一致。将来若要接免费池或云池，各自开一个 site ID 即可——`providerKeySource` 是 `map[string]func(*Config) []string`，多个 ID 可以指向同一个 `EffectiveAPIKeys()`，配置块不用复制。

**这个冲突今天是潜伏的**：本地 catalog 是 8/7 的旧副本（299 模型，无 `cline-pass` 行）。`Sync` 默认 24 小时 TTL，下次同步后它会显形。

## 模型目录

**不能用 `/api/v1/models`。** 实测该端点无鉴权可读、返回 **446 个模型，其中 `cline-pass/*` 为 0**——那是用量计费池的目录。ClinePass 目录在另一个端点：

```
GET https://api.cline.bot/api/v1/ai/cline/recommended-models   → 200，无鉴权
{ recommended: 4, free: 5, clinePass: 12, clineCloud: 3 }
```

这是 Cline 自家 SDK 用来填模型选择器的端点（`catalog-cline-recommended.ts:162`），结构是 `{bucket: [{id, name?, description?, tags?}]}`。

**当初的结构缺口（已解决）**：`SiteModelsURL` 原本只会把 API 端点尾部改写成 `/models`（`siteModelPathSuffix` 表 + 后缀校验），表达不了 `recommended-models` 这种路径，且响应外层包着四个桶、不是 `{data:[...]}`。现在 `cline-pass` 走一条显式登记的路径（`clinePassModelsPath` + `clinePassBucket` 常量），不套后缀改写。

### 能力字段来自 catalog，不是 live roster（2026-09-22 修复）

live roster 的条目**只有 `id/name/description/tags`，没有上下文窗口和输出上限**。这些字段来自 models.dev catalog——但 catalog 把同一批数据**发布两遍**：

| 视图 | 键 | cline-pass 是否在内 |
| --- | --- | --- |
| 顶层 `models` | 完整 `provider/model` | ❌ 无（该 map 早于 cline-pass 建立） |
| `providers[].models` | 裸模型名 | ✅ 15 个 |

`catalog.Load` **只读顶层**，于是 cline-pass 的 15 个模型在列表里可见、却拿不到任何能力数据。`Provider` 结构体当时连 `Models` 字段都没有，嵌套视图在反序列化阶段就被丢弃。

后果不是"数据不全"而是**真实截断**：能力回退到 `modelMetadata[ModelFamily(id)]` 的内置表，而它是通用默认值——

| 模型 | 内置 registry | 平台实际 | 后果 |
| --- | --- | --- | --- |
| `deepseek-v4-pro` | 8,192 | 384,000 | 客户端 64000 被 `clampOutputTokens` **压到 8192** |
| `qwen3.7-max` | 8,192 | 65,536 | 同上 |
| `mimo-v2.5` | 8,192 | 131,072 | 同上 |
| `mimo-v2.5-pro` | 16,384 | 131,072 | 同上 |
| `minimax-m3` | 128,000 | 512,000 | 同上 |
| `deepseek-v4.1-flash`、`glm-5.3`、`glm-5.3-flash`、`qwen3.8-max`、`muse-spark-1.3-contributor` | **0** | 全有 | 完全无上限可依 |

`clampOutputTokens` 里 `MaxOutputTokens` 只降不升（`capacity.go:83`），所以 registry 的 8192 就是一个客户端无法越过的天花板。

**修复**：`Load` 折叠嵌套视图（顶层优先）、`Provider` 加 `Models` 字段、`max_output_tokens` 落库并一路带到 `ResolvedModel`、`PublishedByActiveSite` 改为从 catalog 取能力而不是假定"列出来的 id 就有已知上限"。实测 catalog 模型集 **368 → 8065**（嵌套视图此前完全没被读取，丢掉的不只 cline-pass），`deepseek-v4-pro` 的 `clamp(64000)` 从 8192 变为 **64000**。

**三个来源条数不一致，且都不是全集**：

| 来源 | 条数 | 性质 |
| --- | --- | --- |
| models.dev `cline-pass` | 15 | 有 ctx/output/tools/reasoning/模态 + 价格 |
| 官方文档页 | 13 | 有参考价，**无上下文/输出上限** |
| live `recommended-models` | 12 | 实际可用的运行时事实；能力与价格全为 0 |

差异：`muse-spark-1.3-contributor` 只在 live；`glm-5.3-flash`、`deepseek-v4.1-flash` 只在 models.dev；**9 个三源共有**。

**已定：目录以 live 为准**（`catalog/site_models.go` 读 `recommended-models` 的 `clinePass` 桶）。理由是这个平台**按人按时刻发模型**——免费池前缀都不统一，订阅池同样在轮换；只有一个来源是「账户此刻实际能调用什么」，另外两个都是快照。models.dev 只用于补能力字段（上下文/输出/模态），价格一律用文档页（见下）。

实现方式：`SiteModelsURL` 给 `cline-pass` 走一条显式路径（不套 `siteModelPathSuffix` 的后缀改写——那会打到 `/api/v1/models`，即 446 个用量计费模型且 `cline-pass/*` 为 0），`parseSiteModels` 按平台分流到桶解析。两条路径都保留「空列表是错误、不是空结果」的既有约定。

## 价格表

已生成 `internal/storage/seed_prices_cline_pass.json`（15 条规则）。

**权威源是官方文档页的 Reference pricing 表**，不是 models.dev。逐行比对 13 条：**11 条与文档页完全一致**，2 条 DeepSeek 不符——而 models.dev 给的值与它自己的 MiMo 行**逐字节相同**：

| 模型 | 文档页 Off-peak | 文档页 Peak | models.dev | 判定 |
| --- | --- | --- | --- | --- |
| `deepseek-v4-flash` | 0.22 / 0.66 / 0.007 | 0.44 / 1.32 / 0.014 | 0.14 / 0.28 / 0.0028 | **= `mimo-v2.5` 的值** |
| `deepseek-v4-pro` | 0.66 / 1.98 / 0.022 | 1.32 / 3.96 / 0.044 | 1.74 / 3.48 / 0.0145 | **= `mimo-v2.5-pro` 的值** |

既不是 peak 也不是 off-peak，是映射错误。按 models.dev 计价会把这两条真实流量**少算约 2 倍**。

**来源分层必须留在文件里**（`_source` 字段）：13 行来自文档页，`glm-5.3-flash` 与 `deepseek-v4.1-flash` 两行**不在文档页**、取自 models.dev（第三方）。`muse-spark-1.3-contributor` 在 live roster 里但两源都没有费率，**故意不写规则**——缺规则会报 unknown，而缺规则不等于零价，编一个会显示一个自信的错误数字。

**峰谷已登记（2026-09-22 修正）。** 文档页给 DeepSeek 两行标了 Peak 列，脚注指向 DeepSeek 官方定价页。**从这里可以读出窗口**：该页原文 *"Peak hours are 01:00 - 04:00 and 06:00 - 10:00 UTC, Monday through Friday, excluding Chinese public holidays"* —— **与 CommandCode / OpenCode Go 是同一套**。此前本节写"不是同一套、窗口未核实"，那是把"脚注指向别处"误读成了"规则不同"，而脚注指向的正是能给出窗口的那一页。

因此 `peakSchedules` 已加入 `cline-pass` 条目（×2，同窗口）。覆盖模型为 `deepseek-v4-flash`、`deepseek-v4.1-flash`、`deepseek-v4-pro`——**两个 Flash 拼写都要列**，因为文档页那一行叫 "DeepSeek V4 Flash"，而 live roster 供的是 `cline-pass/deepseek-v4.1-flash` 且没有普通 v4-flash；DeepSeek 官方页把两者接起来（*"the legacy names deepseek-v4-flash ... are still accepted, but the corresponding models have been retired, their requests are served by the DeepSeek-V4.1-Flash model and billed at the Flash price"*）。

中国法定节假日**已建模**（2026-09-22 补）。日历取自 [NateScarlet/holiday-cn](https://github.com/NateScarlet/holiday-cn)（每日抓取国务院公告，MIT，带 `papers` 溯源字段），运行时逐日拉取、24h 刷新，另内嵌一份种子到二进制里兜底。抓取失败保留上一份、绝不清空——清空会把节假日变回工作日，正好是这次要修的错。

**豁免按平台分列**（`peakSchedule.excludesHolidays`），依据是各平台对自己的表述：DeepSeek 官方页写明排除；ClinePass 脚注该页，随之继承；OpenCode Go 自己那句窗口没写豁免，但它的 "Learn more" 指向**同一张页面**，按继承处理（保守方向）；**CommandCode 自己写全了规则且全站不引用 DeepSeek，因此不豁免，节假日照计高峰**。

调休补班日对判定无影响：高峰条件是周一~周五，而调休补班永远是把周末变成工作日，`weekday` 仍非 Mon–Fri。2007–2026 全部 138 个调休上班日中落在周一~周五的只有 1 个（2020-02-03，疫情延期通知里的"正常上班"日，本就不是假日）。

**未登记的代价**：在此修正前，cline-pass 全部请求按 Off-peak 平计。本实例恰好跑 `cline-pass/deepseek-v4.1-flash`，正是被漏掉 peak 率的那个模型，于是高峰时段既不显示角标、费用也少算一半。

### 定时刷新会丢规则（2026-09-22 修复）

`InstallPrices` 原本**整表替换**，而文档页只列它自己宣传的 13 行，seed 的 15 行里有 2 行（`glm-5.3-flash`、`deepseek-v4.1-flash`）来自 models.dev —— 于是**每小时一次的成功刷新会把这两行抹掉**。丢掉一条规则的后果不是报错，是**成本显示为「未知」**，这与"平台没有价格表"给出的答案完全相同，所以表面上什么都看不出来。

这是生产上 cline-pass 全部 66 条请求 `cost_usd` 为 NULL 的直接原因：进程启动 2 秒后日志出现 `prices refreshed provider=cline-pass rules=13`，而 seed 是 15 条。

改为**按 match 合并**（`mergePriceEntries`）：抓到的规则逐条覆盖，只在 seed 里存在的保留。同一缺陷也作用于 CommandCode（`longcat-2.0:free` 已不在 plans 页上）。守卫：`TestRefreshKeepsSeedRulesThePageDoesNotList`（已证实在未修复代码上失败）、`TestRefreshOverridesSeedRateForTheSameModel`（反向：抓到的必须赢过 seed）、`TestMergePriceEntries`。

## 池的区分是模型字符串本身（2026-09-22 实测）

`model` 字段必须是 `modelType/model` 两段式，**前缀就是计费池的选择器**，不是命名空间装饰。实测（同一 key、同一 prompt、交替发送，读 `/users/{id}/balance`）：

| 发送的 model | balance 变化 | five_hour |
| --- | --- | --- |
| `cline-pass/deepseek-v4.1-flash` | **0** | 不变 |
| `deepseek/deepseek-v4.1-flash` | **−305 / 次** | 不变 |

交替四次，只有厂商前缀那两次扣了余额（498459 → 498154 → 497849），订阅前缀那两次完全不扣，且 15 秒静置后无延迟结算。两条路径都返回 200 与同一个 `"model":"deepseek/deepseek-v4.1-flash"`，所以**响应无法区分走了哪个池**——只有账单能。

几个由此确定的事实：

- `deepseek-v4.1-flash` 用厂商前缀调用**不消耗 ClinePass 订阅**，走余额计费。
- 反过来，`cline-pass/` 前缀不会扣余额，也就是**必须带这个前缀才吃订阅额度**。
- **余额单位是 1e-6 credit**：`499797` 对上仪表盘显示的 `Credits: 0.5000`。本次实验共扣 1948 单位 ≈ **0.0019 credit**。
- `cline-free/deepseek-v4.1-flash` 返回 500（免费池对这个模型不可用），`not-a-channel/whatever` 返回 404 `model not found` —— 前缀是会被校验的真实路由键。
- 裸名 `deepseek-v4.1-flash` 返回 400 `invalid model format. Expected format: modelType/model`。**这条决定了本项目的实现必须保持 `model` 原样透传**：`internal/provider/cline_pass.go` 直接把 `model.ModelID` 发上去，`TestClinePassForwardsModelIDUnchanged` 钉住它。若剥掉前缀，上游会 400。

**不写死渠道。** `cline-pass/` 前缀本身**就是**计费池选择器，它已经是写死的（配置里 `model_overrides` 把它固定成 `cline-pass/deepseek-v4.1-flash`）。上游渠道（`finalProvider`）另有一层，但**钉不住**——见「上游渠道」节。

⚠️ **本文档先前声称"响应里没有任何渠道字段"，那是错的。** 该结论来自一份 compactgate 抓包，它之所以干净，是因为抓的是**经过本项目 transformer 之后**的 SSE，而 `provider_metadata` 在转换中被丢弃；且那次成功调用走的是 **Claude Code 直连 `opencode.9962510.xyz` 的实验路径，根本没经过本项目**（另一份 `api.cline.bot` 直连抓包是 404）。**上游原生响应确实带 `provider_metadata.gateway.routing`**，含 `finalProvider`、15 个 `fallbacksAvailable`、`planningReasoning`；本项目不把它透传（`/v1/messages` 只返回 `id/type/role/content/model/stop_reason/usage`），所以从本项目的输出里看不到它。

⚠️ 上面的 `deepseek/` 前缀消耗余额是**本次实验造成的真实扣费**（约 0.0019 credit）。个人自用账户上验证，非生产流量。

## 成本语义：参考消耗，不是账单

ClinePass 是包月，用户**不按参考价付费**。文档页原文：

> ClinePass is a flat monthly subscription, so you are not charged the individual API prices below. These reference prices show the underlying per-1M-token rates for each model and can help you understand how usage is measured against your ClinePass quota.

所以本项目的 `cost_usd` 在这里是**"按参考费率的消耗估算"**，正好就是文档说的那个用途。

这也决定了面板不能照抄 CommandCode 的账本视图：`attachCommandCodeLedger` 做的是"本实例金额 vs 官方金额"，而 ClinePass 该做的是**"参考费率消耗 vs 官方窗口 `percentUsed`"**——后者本身就是百分比，比金额对账更直接。且它的窗口（滚动 5h / 日历周 / 日历月）比 OpenCode Go 的 31 天订阅周期**更好对齐**。

## 配额

`GET https://api.cline.bot/api/v1/users/me/plan/usage-limits`，`Authorization: Bearer <同一个 key>`，**不需要 OAuth**。

响应形状此前由 5 个独立第三方项目交叉证实 + [CodexBar](https://github.com/steipete/CodexBar) 参考实现；**2026-09-22 已在真实 key 上直接验证**：

```json
{"data":{"limits":[
  {"type":"five_hour","percentUsed":1,"resetsAt":"2026-09-21T22:10:02.898904154Z"},
  {"type":"weekly","percentUsed":0,"resetsAt":"2026-09-28T17:10:02.900798633Z"},
  {"type":"monthly","percentUsed":0,"resetsAt":"2026-10-21T17:10:02.902781409Z"}]},
 "success":true}
```

与本项目 `clinePassLimits` 的解析结构逐字段一致（含纳秒精度 `resetsAt`）。`type` ∈ `five_hour` | `weekly` | `monthly`；未知 `type` 跳过而不是报错。

### 百分比的分母从哪来（2026-09-22 新增）

`percentUsed` 是整数百分比（观测值 0/1/2/5/8），**光有它无法判断"还剩多少"**。分母在另一个端点：

```
GET /api/v1/users/me/plan   →  data.plan.entitlements.cline_pass.inferenceCapThreshold
```

```json
{"last5HoursUsageCostUSDPerUser": 1000000000,
 "last7daysUsageCostUSDPerUser":  2500000000,
 "last30daysUsageCostUSDPerUser": 5000000000}
```

**单位是 1e-8 美元**，与账号 usage 记录里的 `costUsd` 同单位。这个换算不是猜的，有三重实测吻合：

| 验证 | 结果 |
| --- | --- |
| 逐条对账 | 12 条 usage 记录的 `costUsd` ÷ 我们按参考费率的估算，**比值恒为 1e8**（误差 <0.005%） |
| 窗口求和 | `∑costUsd` 折成美元后除以阈值，得 5.322% / 2.129% / 1.064%，`floor` 后正是端点报的 **5 / 2 / 1** |
| 折成金额 | 三个阈值 = **$10 / $25 / $50** |

所以本轮实测同时确认了：**我们的参考价与 Cline 自己的计费口径精确一致**，以及配额窗口确实是滚动 5 小时（拉到的 498 条记录跨度 1.50 小时，正好是窗口起点到现在）。

**面板不替换百分比。** `percentUsed` 是平台自己算的权威值，保持为主数字；分母只用来补充一行低调的剩余金额（`$9.47 left of $10.00`）。分母缺失时**不渲染金额**——按百分比倒推会编造平台从未给出的数字。



**仪表盘页面用 cookie 认证，不是 Bearer key。** 从 `app.cline.bot/dashboard/subscription` 抓到的同一请求只带 `cookie`，无 `authorization` 头（页面用 key 直接打会 401）。**但 Bearer key 路径独立可用**——上面的响应即由 key 取得，本项目无需浏览器会话。

**不做的**：`/users/{id}/usages` 虽然逐请求可用（含 `aiInferenceProviderName`、`model_properties_override`），但**只保留当前 5 小时窗口**，日/周/月无法求和。用它做常驻对账既拉不到需要的跨度，又引入分页不稳定（`total` 字段恒为 0）和速率风险，而它本要解决的问题——参考价是否漂移——已由上表的逐条对账一次性验证。故不实现。

**未决**：

- [issue 13707](https://github.com/cline/cline/issues/13707) 的"5 小时锚定首笔请求"未复现：实测同一会话内连续调用，`five_hour.resetsAt` 持续漂移（`…02.787` → `…03.462`），与官方 rolling 说法一致。`percentUsed` 已是百分比、`resetsAt` 已是时间戳，**两种语义都不影响代码骨架**，只影响面板标注，故直接透传不自行推断。
- 企业 REST（`/users/{id}/usages`、`/api/v1/api-keys`）是否对个人 key 开放未验证；本轮不依赖它。

## 已知风险与未验证项

| 项 | 状态 | 影响 |
| --- | --- | --- |
| **非流式行为** | 第三方客户端（OmniRoute、cline2api-workers、cline-pass-switcher-go）报告 `stream:false` 返回空 body 或 `generateText is not implemented`。**2026-09-22 实测修正**：`stream:false` 本身可用，500 `empty response content` 的真实成因是 **`max_tokens` 太小被 reasoning 吃光**（`max_tokens=16` 时 8 次里 6 次失败；`max_tokens≥32` 稳定 200）。见「上游渠道」节末的表格。本实现**始终向上游发 `stream:true`** 并本地聚合 SSE，正好绕开该失败模式 | 无需改动：客户端仍拿到完整 JSON。若将来要放开非流式，必须同时确保 `max_tokens` 足够大 |
| **Anthropic 端点是否存在** | 未能证实。`/api/v1/messages` 返回 401，但**不存在的路径也返回同一个 401**（网关级统一拦截），无法离线区分 | 若无，则 `wire_format` 只能是 `openai`；不影响本设计（已按纯 Chat Completions 设计） |
| **429 响应体** | 网关层会返回**裸 HTML 429**（非 JSON body），且窗口 code 会漂移（同一分钟内 `5-HOUR` ↔ `WEEKLY` 跳变） | 错误解析必须容忍非 JSON；**熔断与配额判断不得依赖窗口 code** |
| **客户端身份门控** | 缺 `X-CLIENT-TYPE` 会得到 `403 "only available via Cline product surfaces"`（cline2api-workers 实测；Cline 官方 PR 13593 记录了自家 commit-message 路径漏发 header 时的同一 403）。门控针对**免费池**；订阅池（`cline-pass/`）未见门控，但本实现**默认带全套官方 header** | 成本最低的兼容策略；版本号当前不做最小值校验（`0.0.1` 实测可过），全套是为了防上游收紧 |
| **上游是 Vercel AI Gateway** | **已由实测证实**（2026-09-22）：非法 `provider.sort` 会返回 Vercel 自己的错误 `failed to invoke model 'deepseek/deepseek-v4.1-flash' from Vercel: ... "param":"provider.sort"` | 解释了配额燃烧波动大。响应 `provider_metadata.gateway.routing` 会公布 `finalProvider` 与 `fallbacksAvailable`（15 个候选渠道），但**钉住无效**——见「上游渠道」节 |
| **ToS 张力** | 官方文档允许第三方调用；ToS §2.2(10) 禁止"非官方技术手段" | 个人自用风险低；**做成多用户/共享/高并发代理会同时踩 §2.2(5)、§7.3(c)(v)、§2.2(7)**。本项目定位是单人自用 |
| 上下文/输出上限 | **已解决（2026-09-22）**：模型列表本身仍不带这些字段，改由 catalog 的嵌套视图提供并已落库（见「模型目录」）。此前 registry 默认值把 `deepseek-v4-pro` 的输出上限从 384,000 压到 8,192 | 不再是未验证项；`max_output_tokens` 现在参与 `clampOutputTokens` |
| `cline-pass` 是否会被 models.dev 调整 | 其 DeepSeek 两行已证有误 | 只当能力字段来源，价格不取它 |

## 上游渠道（uplink）：网关确实公布了，但钉不住

[dsh-cline-pass](https://github.com/yhshzh/dsh-cline-pass)（MIT，dsh 插件，8656 行 JS）实现了完整的「探测 → 校验 → 钉住」链路。它的机制与**在本账户上的实测结果**如下。

### 机制（读代码）

| 步骤 | 实现 | 位置 |
| --- | --- | --- |
| 探测 | 发一条 `provider.only: ['__probe__']` 的请求，**故意让路由失败**，从错误文本里刮出网关本可使用的全部 provider | `protocol.js:192` `extractAvailableProviders` |
| planner 管道 | 错误里刮 `Available providers are: a, b, c` | 同上 |
| direct 管道 | 解析错误 JSON 的 `error.metadata.available_providers` | 同上 |
| 钉住 | 往请求体注入 `provider.only=[...]` / `provider.order=[...]` / `provider.sort` | `protocol.js:98` `injectPrefs` |
| 两种拼写 | direct 用顶层 `provider`；planner（Vercel）用 `providerOptions.gateway` | 同上 |
| 读回 | 从响应的 `provider_metadata.gateway.routing` 取 `finalProvider`、`fallbacksAvailable`、`planningReasoning` | `protocol.js:42` `parseRouting` |
| 排他 | 网关忽略 exclude 字段，所以排除被编译成 `only` 白名单 | `protocol.js:106` |
| 失败学习 | 从错误文本 `Available providers are:` 反推渠道表 | `engine.js:146` |

### 在本账户上的实测：**元数据是真的，钉住是空操作**

`provider_metadata.gateway.routing` **确实存在且信息量很大**（这纠正了本文档先前"响应里没有任何渠道字段"的说法——那个结论来自一份截断的 SSE 抓包，不是全量）：

```json
{"routing":{"canonicalSlug":"deepseek/deepseek-v4.1-flash",
  "fallbacksAvailable":["alibaba","baseten","fireworks","runware","relace","particle",
                        "novita","togetherai","deepinfra","wafer","parasail","gmicloud",
                        "modal","morph","boundless"],
  "finalProvider":"deepseek",
  "planningReasoning":"System credentials planned for: deepseek, alibaba, baseten, ..."}}
```

但**注入 `provider.only` / `provider.order` / `providerOptions.gateway.only` 全部无效**：

| 测试 | `finalProvider` |
| --- | --- |
| 裸调用 ×6 | `alibaba` ×6 |
| `provider.only=["novita"]` ×6 | **`alibaba` ×6** |
| `providerOptions.gateway.only=["novita"]` ×6 | **`alibaba` ×6** |
| `provider.only=["deepseek"]` | `alibaba` |
| `provider.only=["zzz-nope"]`（不存在的渠道） | `alibaba`，**仍 200** |

`cline-pass/` 前缀的模型上同样：一律 `deepseek`，无视钉住。

**但该字段确实被解析**——`provider.sort` 传一个非法值会得到 Vercel 的 400：

```
400 {"error":{"message":"Invalid option: expected one of \"cost\"|\"ttft\"|\"tps\"|\"price\"|\"latency\"|\"throughput\"","param":"provider.sort"}}
```

所以 `provider` 块**到达了 Vercel AI Gateway 并被校验，只是路由没有遵循 `only`**。与本文档早先记录的"上游是 Vercel AI Gateway（创始人自述，未独立验证）"吻合，且这次由错误文本 `failed to invoke model ... from Vercel` 直接证实。

结论：**渠道探测在本账户上不可行**，因为它的前提（不可能的 `only` 会让路由失败并报出渠道全集）不成立——路由忽略 `only`，请求照常成功。插件作者显然是在一个 `only` 生效的环境里开发的（或上游后来改了行为）。第二个探测路径（从错误文本刮 provider 列表）同样落空。

**不实现渠道钉住。** 本项目不引入一个在目标账户上被证明是空操作的功能；渠道由网关自行选择，`fallbacksAvailable` 等字段只用于观测，不进路由决策。

### 与当前模型的关系

| 插件 | 本项目 | 说明 |
| --- | --- | --- |
| `catalog.js` 硬编码 **15** 个 `cline-pass/*`（含 `glm-5.2`、`kimi-k2.7-code`、`kimi-k2.6`、`deepseek-v4-flash`） | live roster **12** 个 | 插件的目录是快照；其中 4 个已不在 live `clinePass` 桶里。本项目以 live 为准 |
| 模型 URI 用 `cline-pass/` 前缀 | 同 | 两边都保留前缀，理由一致：剥掉会被上游 400 |
| 网关目录拉 `GET {baseURL}/models`（`cline.js:102`） | 拒绝该端点 | 实测 440 条、`cline-pass/*` **为 0**——那是按量计费池。本项目读 `recommended-models` 的 `clinePass` 桶 |
| 非流式：`chatCompletion` 不带 `stream` | 强制 `stream:true` 后本地聚合 | 见下 |

**非流式结论修正。** 本文档先前写"上游只可靠地支持流式"。实测：`stream:false` **可用**，500 `empty response content` 的真实成因是 `max_tokens` 太小。

| `max_tokens` | `stream:false` 结果 |
| --- | --- |
| 8 | 500 `empty response content` |
| 16 | **不稳定**：8 次里 6 次 500、2 次成功（`completion_tokens=16`，即正好顶满） |
| 32 | 稳定 200 |
| 64 / 128 / 256 / 512 | 稳定 200 |

原因不是流式，而是 **reasoning 先吃掉预算**：`max_tokens:16` 且 `stream:true` 时，5 次里有 4 次是 16 个 `reasoning` 分片、**0 个 `content` 分片**——推理把预算耗尽，正文没开始就结束，非流式路径于是报 `empty response content`。deepseek-v4.1-flash 是推理模型，小预算必然先烧在思考上。

**本项目的实现不需要改**：始终向上游发 `stream:true` 并本地聚合，正好绕开了这个失败模式，且客户端拿到的仍是完整 JSON。这条实测把原来的理由（"上游不支持非流式"）修正为更准确的（"上游支持，但小 `max_tokens` 下非流式会把 reasoning 耗尽误报成空响应"）。

## 接入步骤（照 CommandCode 逐处对照）

| 阶段 | 内容 | 关键文件 | 状态 |
| --- | --- | --- | --- |
| 0 | 用真 key 实测：非流式、429 形状、`usage-limits` 实值、header 是否必需 | — | **部分完成（2026-09-22）**。已实测：`usage-limits` 真实响应（见「配额」）、模型前缀即计费池选择器（见「池的区分」）、推理链路端到端跑通（真实 key、`active_site=cline-pass`、200 + 有效 SSE）。仍未实测：429 裸 HTML 的真实形状、header 是否真的必需（`X-CLIENT-TYPE` 缺失时的免费池门控未复现，订阅池未见门控） |
| 1 | site descriptor + `ClinePassConfig` + loader（env/validate/siteDefaults）+ 两处 CLI 清单 | `internal/site/site.go`、`internal/config/config.go`、`loader.go`、`cmd/routatic-proxy/init_provider.go`、`main.go` | **已完成**。全量测试通过，既有断言未修改 |
| 2 | provider 实现 + 注册 + 非流式聚合 | `internal/provider/cline_pass.go`、`cline_pass_stream.go`、`internal/server/server.go`、`internal/client/headers.go` | **已完成** |
| 3 | 目录：显式 URL + 桶选择 | `internal/catalog/site_models.go` | **已完成** |
| 4 | 价格：`FetchClinePassPrices` 解析文档页两张表 → `rateTableFiles` 加一行 | `internal/storage/pricerefresh.go`、`database.go` | **已完成**。seed 与实时页一致性由测试钉住 |
| 5 | 配额 `internal/quota/clinepass.go` + 面板区块 + 文档 | `internal/gui/quota.go`、`assets/index.html`、`assets/app.js`、`docs/` | **已完成** |

**不做的**：不扩 `site.Descriptor`（`docs/site-architecture.md:184-191` 已论证六字段描述符会引入导入环、且把编译期检查换成运行期检查）；不碰 `/v1/models` 那 446 个用量计费模型（前缀是厂商名，会与 models.dev 的 `anthropic`/`openai` 等 provider 撞名，是另一个问题）。

**provider 实现的三个必带项**（均已落地并有测试）：

1. `stream` 字段始终显式写入——Cline API 的 `stream` **默认 `true`**，省略即等于请求流式。
2. **非流式路径强制上游流式并本地聚合**。客户端侧 `stream` 标志不被改写：上游收到 `stream:true`，调用方仍得到一份完整 JSON（聚合上限 64 MiB）。聚合按 index 拼接 `tool_calls` 的 arguments 分片、按最后一块取 usage——这三处都是流式才有的形态。
3. 官方客户端 header：`HTTP-Referer: https://cline.bot`、`X-Title: Cline`、`X-IS-MULTIROOT: false`、`X-CLIENT-TYPE: cline-sdk`（与 Cline 源码 `request-headers.ts` 的 `DEFAULT_CLINE_REQUEST_HEADERS` 一致），走 `client.SetProviderHeaders`。

另：错误解析对**非 JSON 响应体**显式降级（网关会返回裸 HTML 429），JSON 错误体则原样透传不重写。

另：ClinePass 只有 Chat Completions，所以 `tool_reference` 丢弃问题与 CommandCode 非 Claude 模型完全相同（见 `docs/commandcode.md`），不是新问题。

## 验证

- 阶段 1：全量 `go test -race -p 2 ./... -count=1` 现有断言原样通过（零行为变化的判据）。
- 阶段 3：`active_site=cline-pass` 时 `/v1/models` 返回 `cline-pass/*`；拉取失败必须是错误响应而非空列表。
- 阶段 4：对文档页 13 行做一次价格回放断言；隔离实例核对 `requests.provider=cline-pass` 与逐条成本。
- 阶段 5：`usage-limits` 的三个窗口与面板显示一致；429 裸 HTML 不导致解析 panic。
- 面板：沿用既有 Edge 只读验收脚本。

**未验证**：真实账户扣费、`percentUsed` 与本地消耗的换算关系、非流式是否真的可用、Anthropic 端点是否存在、所有模型与客户端版本。编译通过不等于这些能力全部兼容。
