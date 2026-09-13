# 站点可插拔架构

设计定稿：2026-09-13。状态：**阶段 0–4 已实施**，阶段 5（清理残留平台分支）未开始。

已实施：`internal/site` 注册表与重复清单收敛（0/1，零行为变化）；面板只保留 `opencode-go` 与 `commandcode`，隐藏项保留代码与历史（2）；站点自有模型目录接入，修好 `/v1/models` 对 CommandCode 返回 0 项（3）；`active_site` 作用域 + 目录优先解析 + 面板选择器（4）。

生产配置不在本设计范围内改动，由操作者自行修改；`active_site` 未设置时路由行为与从前完全一致。

本文只描述目标架构与推进方式，不描述当前实现细节；当前实现见各包内的代码与注释。

## 1. 目标与非目标

**目标**

- 一个站点（上游平台）的身份、凭证、端点、协议适配、计费规则、账户接口、模型目录收敛到**一个描述符**；新增站点不改其他包。
- 面板可切换「当前站点」，切换后 `/v1/models` 自动变为该站点的真实模型列表，客户端无需感知。
- 前端只保留 `opencode-go` 与 `commandcode`，其余站点隐藏但代码与历史数据保留。

**非目标**

- **不做跨站点轮询、健康记忆、自动兜底。** 当前站点就是唯一目标；站点不可用时由操作者手动切换。
- **不做跨站点模型等价解析。** 站点与模型的匹配由客户端选择与上游响应决定，本项目不改写。
- **不管理模型。** 本项目只负责：协议适配（站点若不原生提供 Anthropic / OpenAI 接口，则转换请求与响应）、缓存相关字段处理、用量与费用记账。
- 不把站点行为做成 JSON DSL。新增行为特殊的站点仍需一个 Go 文件。

## 2. 现状：平台身份散落在哪

`core.Provider` 接口本身是正确的抽象，问题在于**元数据没有归属**，于是每个关注点各写一份。清单如下，覆盖 19 个文件：

| 关注点 | 位置 |
| --- | --- |
| 平台白名单 | `config.SupportedProvider`（`internal/config/config.go:349`） |
| 默认平台与拼写归一 | `config.NormalizeProvider`（`internal/config/config.go:336`） |
| 凭证与全局回退规则 | `config.ProviderAPIKeys`（`internal/config/config.go:361`）——五臂 switch，Go/Zen/Bedrock/OpenRouter 回退全局 key，CommandCode 不回退 |
| 展示顺序 | `providerDisplayRank`（`internal/config/provider_display.go:12`） |
| 配置结构 | 五个类型化 struct（`config.go:137/185/211/235/257`） |
| 端点默认值 | `defaultAnthropicBaseURL` 等常量与各自的空值分支（`internal/config/loader.go:21/30/35/36`） |
| 峰谷计费 | `history.ProviderPeakMultiplier`（`internal/history/record.go`） |
| 未知模型的兜底 provider | `legacyUnknownModelConfig` 硬编码 `opencode-go`（`internal/router/model_router.go:217`） |
| 模型列表来源 | `ModelRouter.ListModels`（`internal/router/model_router.go:396`） |
| 成本路由的平台偏好 | `internal/router/selector.go` 的 `prefer_providers` |
| 定价分支 | `isGo := provider == "" \|\| provider == "opencode-go"`（`internal/storage/pricing.go:36`） |
| 用量同步目标 | `providerUsageTarget`（`internal/storage/provider_request_sync.go:15`） |
| 存储默认 provider | `internal/storage/requests.go`、`internal/storage/database.go` |
| 账户与配额 | `internal/quota/` 各平台文件、`internal/gui/quota.go:128`、`internal/gui/analytics.go:78` |
| 面板平台清单 | `internal/gui/config_io.go`、`internal/gui/assets/app.js` |
| CLI 预设 | `cmd/routatic-proxy/init_provider.go`、`cmd/routatic-proxy/main.go` |
| 遗留客户端 | `internal/client/opencode.go` 的 `Provider`（`getEndpoint` / `IsZen` / `IsBedrock` / `IsOpenRouter` **保留**，见文末） |
| 协议实现 | `internal/provider/*.go` —— **这条已经是对的**，保持 |

## 3. 目标架构

### 3.1 站点描述符

新增 `internal/site`。**元数据是值，行为是代码。**

```go
type Site struct {
    ID          string // "opencode-go" —— 写进 requests.provider 的就是它
    DisplayName string
    Order       int    // 面板顺序，取代 providerDisplayRank
    Visible     bool   // false = 前端隐藏，代码与历史保留

    Credentials CredentialSpec  // 配置字段、环境变量名、是否回退全局 key
    Endpoints   []Endpoint      // {Name, ConfigField, Default, EnvOverride}
    Provider    core.Provider   // nil = 该站点走通用 OpenAI 兼容路径
    Peak        PeakRule        // nil = 无峰谷
    Account     AccountReader   // nil = 无账户/套餐页
    Catalog     CatalogSource   // 模型目录来源，见 §5
}
```

注册点只有一处，与今天注册 provider 的位置相同（`internal/server/server.go`）。其余包一律 `site.Lookup(id)`，不再各自写 switch。

**「可配置附加」的含义**：新增一个站点 = 一个 `internal/site/sites/<name>.go` + 一段配置，不改配置结构体、不改 switch、不改前端。长尾的纯 OpenAI 兼容站点用 `site.GenericOpenAICompatible(...)` 覆盖，不需要专属文件。

### 3.2 职责边界

这条边界决定改造是否会做过头，必须先划清：

| 规则 | 归属 | 理由 |
| --- | --- | --- |
| 上游协议选择（Anthropic / Chat Completions / Responses / Gemini） | **站点** | 同一模型在不同站点走不同端点 |
| 站点若不原生提供 Anthropic / OpenAI 接口，则转换收发格式 | **站点** | 本项目在这里的角色就是代理 |
| 缓存字段语义（`cache_control` 是否有效、是否剥离） | **站点** | 站点网关认不认该字段，是站点的事 |
| 峰谷时段与倍率 | **站点** | 窗口可能相同，但覆盖的模型集合不同 |
| 账户 / 配额接口、模型目录来源 | **站点** | |
| `thinking` / `reasoning_effort` 字段 | **模型家族** | 模型厂商的 API 契约 |
| temperature 约束、`reasoning_content` 占位 | **模型家族** | 同上 |
| system 消息重写以保住前缀缓存 | **模型家族** | 由模型内部重排序行为决定 |
| 模型是否有货、套餐是否覆盖 | **上游** | 本项目不预检，上游返回什么就是什么 |

上一条不是"暂时没做"，而是做不了：站点返回的模型列表不按套餐分档。CommandCode 对任何套餐都返回同一份 `/v1/models`，所以列表里出现某个模型，并不代表当前套餐能用它——拿列表做预检只会把"套餐不含"错判成"可选"。这也是 `claude-opus-4-7` 这类请求在 `active_site: commandcode` 下直接 502 的原因：本项目照实转发，由平台回 `403 MODEL_NOT_IN_PLAN`。选错模型报错是预期行为，不加兜底。

即 **行为 = f(站点, 模型家族)**，两边各自只声明自己那半。模型家族判定由 `internal/models.ModelFamily` 统一提供，不要挪进站点描述符。

### 3.3 请求解析

采用**目录优先**：

1. 请求里的 model id 若命中**当前站点**的模型目录（`CatalogSource` 的实时结果），直接以该 id 路由到当前站点。
2. 否则走现有配置链（`model_overrides` → `model_family_overrides` → `respect_requested_model` → 场景路由），结果**只保留 `provider == active_site` 的目标**。
3. 两者都得不到目标 → 显式报错，写明当前站点与可用模型数。

第 3 条的边界要说清楚：**「无目标」指的是配置把该请求指向了别的平台**，不是模型名陌生。模型名没有任何来源认识时，它仍被送到当前站点、由平台自己回答——本项目不管理模型，也不替平台预判某个模型是否存在或是否有权限。这与「客户端从目录里选了什么就用什么」是同一条原则。

不做跨站点等价替换。当前站点是解析作用域，不是覆盖之上的覆盖。

**已知代价**：目录优先意味着，当客户端发来的名字同时存在于当前站点目录和 `model_overrides` 里时，**配置里的重映射意图会被目录优先覆盖**。例如 `claude-opus-4-7` 在 CommandCode 目录中存在，而配置把它重映射到某个 DeepSeek 模型；当前站点为 CommandCode 时，目录优先会使该重映射失效。这是本设计接受的代价——模型如何选择由客户端与上游决定，本项目不改写。

### 3.4 当前站点

配置字段 `active_site`，热重载生效，重启后保持。无凭证的站点不可选、不可切（面板不列出）。无兜底：站点不可用时请求失败，由操作者切换。

## 4. 面板

- 站点选择器只列 `Visible && 有凭证` 的站点。
- 切换写入 `active_site`，触发配置热重载；不为任何站点存储模型清单。
- 切换后 `/v1/models` 立即反映新站点的目录，客户端重新拉取即可，无需感知站点变化。
- 面板不得显示与实际路由不符的状态：若当前站点下没有任何目标可用，必须显式提示，而不是显示「正常」。

## 5. `/v1/models` 与站点目录

**这是当前的实际缺陷。** `ModelRouter.ListModels`（`internal/router/model_router.go:396`）只有三个来源：`cfg.ModelOverrides` 的键、`cfg.Models` 的键、models.dev catalog 的规范名。CommandCode 不在 models.dev 中，因此生产 362 项里 CommandCode 为 **0 项**——面板切过去模型选择器是空的。

每站点目录来源：

| 站点 | 来源 | 现状 |
| --- | --- | --- |
| CommandCode | 由 `commandcode.base_url` 推导 `/provider/v1/models` | 69 个模型，**无需鉴权**（2026-09-13 实测）。推导方式与 `internal/quota/commandcode.go` 现有的 `/provider/v1` → `/alpha` 同源 |
| OpenCode Go | models.dev catalog | 已有 |
| 其他 | 各自的公开端点，没有则用配置别名 | |

约定：目录**不落盘存储**，按 TTL 内存缓存；**拉取失败必须报错，不得返回残缺或空列表**——空列表会被客户端读成「该站点没有模型」，与「没拉到」是两回事。

## 6. 分阶段推进

| 阶段 | 内容 | 行为变化 | 回退 |
| --- | --- | --- | --- |
| 0 | 固化 §2 清单为检查项，确认无遗漏 | 无 | — |
| 1 | 建 `internal/site`，把白名单、默认值、凭证规则、展示顺序、端点默认值改为查描述符 | **无**（纯搬移，现有测试必须逐项原样通过） | 单提交 |
| 2 | `app.js` 与面板按 `Visible` 过滤，其余站点置 `false` | 前端少若干项，路由与数据不变 | 改一个布尔值 |
| 3 | 接入每站点目录，修好 `/v1/models` | 仅影响模型列表内容 | 独立提交 |
| 4 | `active_site` + 目录优先解析 + 面板选择器 | **唯一改变路由行为的阶段** | 移除 `active_site` 即回到阶段 3 |
| 5 | 清理 `provider/*.go`、`loader.go`、`client/opencode.go` 中残留的平台分支 | 无 | 已完成 `loader.go` 与 `storage/pricing.go`；`client/opencode.go` 经核实**不应清理**，见下 |

**阶段 1 是承重点**：它必须零行为变化。做不到零变化说明描述符的边界划错了，应停下来重新划线而不是继续往上叠。

## 7. 验证

- 阶段 1–2：现有全量 `go test -race -p 2 ./... -count=1` 与面板行为测试**不得新增或修改断言**，全部原样通过即为判据。
- 阶段 3：`/v1/models` 在 `active_site=commandcode` 时必须返回该站点的真实模型；拉取失败时必须是错误响应。隔离实例上实测，不依赖真实账户推理请求。
- 阶段 4：三组断言——目录命中优先于配置重映射；未命中目录时按配置链过滤到当前站点；两者都不命中时是显式错误。用隔离实例跑真实双站点请求，核对 `requests.provider` 归属。
- 面板：沿用现有 Edge 只读验收脚本，增加站点选择器与「当前站点下无可用目标」提示的断言。

## 8. 已知代价与风险

| 项 | 说明 |
| --- | --- |
| 可用性责任转移 | 无兜底意味着站点故障 = 服务故障，恢复手段是手动切换。这是选择「不做轮询/不做健康记忆」的代价，不是意外 |
| 切换前需先配好目标 | 切换开关只在目标站点已有可用目标时才有意义。生产今天的 13 个路由目标（`models` 6 + `model_overrides` 5 + `fallbacks.default` 2）全部指向 `opencode-go`，CommandCode 有 key 但零引用，切过去会是空的 |
| 目录优先覆盖重映射 | 见 §3.3，已知且接受 |
| 描述符退化为配置桶 | 硬线：描述符只放**可枚举的元数据**。出现需要 `if` 才能解释的字段时，说明该行为不属于描述符，应放回各自包 |

## 9. 阶段 5 的一处反例：`client.getEndpoint` 不应清理

`internal/client/opencode.go` 的 `getEndpoint` 按平台分支返回端点，其中 Go / Zen / Bedrock 三个分支看起来是死代码——这四个平台都有 `core.Provider` 适配器并已在 registry 注册，handler 只在 registry 查不到时才落到这个客户端。

**但它们是可达的**：`MessagesHandler` 的 `providerRegistry` 允许为 nil（`messages.go` 的 `if h.providerRegistry != nil`），此时所有平台都走这条路径。实际删掉后 `TestHandleStreaming_GoAnthropicModel_FallsThroughOnError` 与 `TestHandleStreaming_PerModelTimeoutFallback` 立即失败，已回退。

结论：这条路径是**被支持的降级配置**，不是残留。要清理它，前提是先决定 registry 为 nil 是否仍是受支持的构造方式；在那之前保持原样。
