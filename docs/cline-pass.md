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

**结构缺口**：`catalog/site_models.go:37` 的 `SiteModelsURL` 只会把 API 端点尾部改写成 `/models`（`siteModelPathSuffix` 表 + 后缀校验），表达不了 `recommended-models` 这种路径，而且响应外层还包着四个桶、不是 `{data:[...]}`（`siteModelsResponse`，`site_models.go:61`）。需要给它加一条**显式目录 URL + 桶选择**的登记方式。

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

**峰谷暂不登记。** 文档页给 DeepSeek 标了 Peak 列，但脚注指向 DeepSeek 官方定价页，**与 CommandCode/OpenCode Go 的 01–04 & 06–10 UTC 不是同一套**。窗口未核实前 `peakSchedules` 不加 `cline-pass` 条目，`PeakMultiplier` 恒为 1、按 Off-peak 平计。**猜一个窗口会产出格式正确、金额错误的成本**——这正是 `history/record.go` 那张表按平台分开存的原因。

## 成本语义：参考消耗，不是账单

ClinePass 是包月，用户**不按参考价付费**。文档页原文：

> ClinePass is a flat monthly subscription, so you are not charged the individual API prices below. These reference prices show the underlying per-1M-token rates for each model and can help you understand how usage is measured against your ClinePass quota.

所以本项目的 `cost_usd` 在这里是**"按参考费率的消耗估算"**，正好就是文档说的那个用途。

这也决定了面板不能照抄 CommandCode 的账本视图：`attachCommandCodeLedger` 做的是"本实例金额 vs 官方金额"，而 ClinePass 该做的是**"参考费率消耗 vs 官方窗口 `percentUsed`"**——后者本身就是百分比，比金额对账更直接。且它的窗口（滚动 5h / 日历周 / 日历月）比 OpenCode Go 的 31 天订阅周期**更好对齐**。

## 配额

`GET https://api.cline.bot/api/v1/users/me/plan/usage-limits`，`Authorization: Bearer <同一个 key>`，**不需要 OAuth**。

响应形状（5 个独立第三方项目交叉证实 + [CodexBar](https://github.com/steipete/CodexBar) 参考实现）：

```json
{ "success": true,
  "data": { "limits": [ { "type": "five_hour", "percentUsed": 42.5, "resetsAt": "<ISO8601>" }, … ] } }
```

`type` ∈ `five_hour` | `weekly` | `monthly`；`percentUsed` 是 0–100 的数字；`resetsAt` 是 ISO8601 字符串（可能为 null）。未知 `type` 跳过而不是报错。

**未决（需实测）**：

- 官方文档说 5 小时是 *rolling*，而 [issue 13707](https://github.com/cline/cline/issues/13707) 的实测显示 `resetsAt` **锚定到首笔计费请求**，weekly/monthly 锚定到账号创建时刻。官方与实测冲突。
- 因为 `percentUsed` 已是百分比、`resetsAt` 已是时间戳，**这两个分歧都不影响代码骨架**，只影响面板怎么标注（"滚动" vs "固定窗口"）。直接透传即可，不自己推断窗口语义。
- 企业 REST（`/users/{id}/usages`、`/api/v1/api-keys`）是否对个人 key 开放未验证；本轮不依赖它。

## 已知风险与未验证项

| 项 | 状态 | 影响 |
| --- | --- | --- |
| **非流式行为** | **已按「上游只可靠地支持流式」实现**。官方文档给出非流式响应示例，但三个独立客户端（OmniRoute、cline2api-workers、cline-pass-switcher-go）的实测与注释一致相反：`stream:false` 返回空 body 或 `generateText is not implemented`。本实现因此**始终向上游发 `stream:true`**，非流式路径本地聚合 SSE 成完整 JSON（`internal/provider/cline_pass_stream.go`）。客户端拿到的仍是完整文档，不是 SSE | 若上游日后修复非流式，这只是多一次聚合，不需要改契约 |
| **Anthropic 端点是否存在** | 未能证实。`/api/v1/messages` 返回 401，但**不存在的路径也返回同一个 401**（网关级统一拦截），无法离线区分 | 若无，则 `wire_format` 只能是 `openai`；不影响本设计（已按纯 Chat Completions 设计） |
| **429 响应体** | 网关层会返回**裸 HTML 429**（非 JSON body），且窗口 code 会漂移（同一分钟内 `5-HOUR` ↔ `WEEKLY` 跳变） | 错误解析必须容忍非 JSON；**熔断与配额判断不得依赖窗口 code** |
| **客户端身份门控** | 缺 `X-CLIENT-TYPE` 会得到 `403 "only available via Cline product surfaces"`（cline2api-workers 实测；Cline 官方 PR 13593 记录了自家 commit-message 路径漏发 header 时的同一 403）。门控针对**免费池**；订阅池（`cline-pass/`）未见门控，但本实现**默认带全套官方 header** | 成本最低的兼容策略；版本号当前不做最小值校验（`0.0.1` 实测可过），全套是为了防上游收紧 |
| **上游是 Vercel AI Gateway** | 创始人自述（Reddit，未独立验证），按可用性与价格负载均衡 | 解释了配额燃烧波动大；对本项目实现无影响，写进文档备查 |
| **ToS 张力** | 官方文档允许第三方调用；ToS §2.2(10) 禁止"非官方技术手段" | 个人自用风险低；**做成多用户/共享/高并发代理会同时踩 §2.2(5)、§7.3(c)(v)、§2.2(7)**。本项目定位是单人自用 |
| 上下文/输出上限 | 官方文档完全未给；第三方三套数字互相矛盾（`mimo-v2.5` 一套 1,048,576、一套 262,144） | 取 models.dev 值并标注为未验证；不用于任何硬校验 |
| `cline-pass` 是否会被 models.dev 调整 | 其 DeepSeek 两行已证有误 | 只当能力字段来源，价格不取它 |

## 接入步骤（照 CommandCode 逐处对照）

| 阶段 | 内容 | 关键文件 | 状态 |
| --- | --- | --- | --- |
| 0 | 用真 key 实测：非流式、429 形状、`usage-limits` 实值、header 是否必需 | — | **未执行**：没有真实 key。非流式与 header 两条已由第三方实现交叉证实（见「已知风险」），`usage-limits` 的真实数值仍未验证 |
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
