# CommandCode、Claude Code 与 Codex 接入

核实日期：2026-09-12。接入使用官方 Provider API，不读取或修改用户的全局客户端配置。

## 原生支持与路径选择

| 调用方式 | 官方合同 | 本项目实现 |
| --- | --- | --- |
| Claude Code → Claude 模型 | 原生 Anthropic Messages | 直接转发到官方 Messages，保留本地日志/统计 |
| Claude Code → 其他模型 | 官方 Messages 不接受非 Claude 模型 | 使用已有 Messages → Chat Completions 转换 |
| Codex → CommandCode | Codex 要求 Responses；官方端点表尚未公布 Responses | 本项目 `/v1/responses` 适配到已有路由及官方 Messages/Chat API |

上述判断来自 [CommandCode Provider API](https://commandcode.ai/docs/provider)、[Codex 配置参考](https://developers.openai.com/codex/config-reference)和 [Claude Code 网关文档](https://code.claude.com/docs/en/llm-gateway-connect)。未列出 Responses 是公开合同缺口，不是使用真实账户测试后得出的“服务端一定不支持”。

推荐路径是客户端 → 本项目 → 官方 API，原生链路没有额外的第三方代理进程。客户端完全绕过本项目直连官方时，本地无法记录那部分流量。

## 独立平台配置

新配置使用 `routatic-proxy init --provider commandcode`。`init` 不覆盖已有文件；已有项目可在 Settings 的 CommandCode 区域填写独立设置，并在模型/降级配置中使用 `provider: "commandcode"`。

单平台最小配置如下；将环境变量设置为自己的平台密钥，不要把真实值提交到仓库：

```json
{
  "host": "127.0.0.1",
  "port": 3456,
  "respect_requested_model": false,
  "models": {
    "default": {"provider": "commandcode", "model_id": "claude-sonnet-4-6", "max_tokens": 8192, "vision": true}
  },
  "model_overrides": {
    "commandcode": {"provider": "commandcode", "model_id": "claude-sonnet-4-6", "max_tokens": 8192, "vision": true}
  },
  "commandcode": {
    "base_url": "https://api.commandcode.ai/provider/v1/chat/completions",
    "anthropic_base_url": "https://api.commandcode.ai/provider/v1/messages",
    "api_key": "${ROUTATIC_PROXY_COMMANDCODE_API_KEY}",
    "api_keys": [],
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000,
    "streaming_timeout_ms": 600000,
    "zero_data_retention": false
  },
  "catalog": {"enabled": false},
  "logging": {"level": "info", "requests": true}
}
```

这里的两个 URL 是完整端点，不是 API 根地址。CommandCode 密钥不会回退到全局 `api_key`；原有四个平台的全局回退仍兼容。仅使用 CommandCode 时不需要全局密钥，也不要留下未设置的旧全局 `${ROUTATIC_PROXY_API_KEY}` 占位符。

| 配置/环境变量 | 行为 |
| --- | --- |
| `api_keys` / `ROUTATIC_PROXY_COMMANDCODE_API_KEYS` | 逗号分隔环境变量用于密钥轮换；池优先于单密钥 |
| `api_key` / `ROUTATIC_PROXY_COMMANDCODE_API_KEY` | 平台单密钥；环境变量覆盖文件 |
| `ROUTATIC_PROXY_COMMANDCODE_URL` | 覆盖完整 Chat Completions URL |
| `ROUTATIC_PROXY_COMMANDCODE_ANTHROPIC_URL` | 覆盖完整 Messages URL |
| `timeout_ms` | 非流式单次尝试超时 |
| `stream_timeout_ms` | 流式无新数据的空闲超时 |
| `streaming_timeout_ms` | 流式单次尝试总超时 |
| `zero_data_retention` | 向 CommandCode 发送 `x-cmd-zdr: 1`，不自动取消该约束 |

`wire_format` 是**上游**协议，不是客户端协议：默认 `claude-` 前缀走 `anthropic`，其他模型走 `openai`；CommandCode 配置不接受上游 `responses` 或 `gemini`。若设置自定义端点/模型别名，可显式选择 `anthropic` 或 `openai`，但官方端点仍会拒绝模型与协议不匹配的请求。

CLI 预设提供 `commandcode`、`claude-sonnet-4-6` 和 `deepseek/deepseek-v4-flash` 映射；默认及 Claude 家族映射到 Sonnet。要固定其他实际模型，请修改对应的 `model_overrides`，不要假设预设包含官方全部模型。公开模型列表可从 [Models 端点](https://api.commandcode.ai/provider/v1/models)查询，它不包含账户额度，也不会自动成为本地价格来源。缺价模型不会被自动成本路由当作免费候选。

运行 `routatic-proxy validate` 后，以 `routatic-proxy start` 启动代理及面板；仅需代理则使用 `serve`。

## Claude Code

经过本项目、保留本地日志：

```sh
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
claude --model commandcode
```

这里的 `unused` 不是平台密钥，真实上游密钥由本项目独立配置提供。本地代理没有入站鉴权，保持 loopback 监听，不直接暴露公网。原生 Messages 会保留请求中的工具定义、cache control、beta 扩展以及 `anthropic-version`/`anthropic-beta`；上游是否接受具体扩展仍取决于平台。

**已知限制：`tool_reference` 在 Chat Completions 上会被丢弃。** Claude Code 的 ToolSearch 会把工具检索结果作为 `tool_result` 里的 `tool_reference` 内容块发回，该形状在 Chat Completions 里没有对应表示。当 `active_site` 指向一个只提供 Chat Completions 的平台（CommandCode 上除 Claude 外的全部模型）时，该块被丢弃，`tool_result` 里可表达的部分（文本）保留；只有 `tool_reference` 的结果会成为一条空的 tool 消息，请求本身成功。

这与参考项目 [MAXeaglet/commandcode-proxy](https://github.com/MAXeaglet/commandcode-proxy) 的处理一致（它把同一块映射为 `""`），是刻意选择的取舍：**请求成功、模型在没有该工具内容的情况下继续**，而不是让整条链因为一个无法表达的块失败。代价是模型可能基于"工具没有返回内容"这个错误前提继续推理，且客户端看不到提示——排障时需知道这一点。要避免该丢弃，需换用原生 Messages 的目标（Claude 模型）。

若不需要本地记录，也可让 Claude Code 直接调用官方 Claude 模型：

```sh
export ANTHROPIC_BASE_URL=https://api.commandcode.ai/provider
export ANTHROPIC_AUTH_TOKEN='<your-commandcode-api-key>'
claude --model claude-sonnet-4-6
```

Claude Code 会追加 `/v1/messages`，所以此处 Base URL 不含 `/v1`。Bearer 和 Messages 的 `x-api-key` 均有官方支持；不要同时保留相互冲突的凭证变量。该直连方式不支持用官方 Messages 调用非 Claude 模型，也不会进入本项目的日志、额度估算或统计。

## Codex

在自己的 Codex 配置中合并以下设置，不要覆盖已有配置文件。客户端模型名使用已验证的 `commandcode` 别名，真实上游模型在本项目中映射：

```toml
model = "commandcode"
model_provider = "routatic"
web_search = "disabled"
model_supports_reasoning_summaries = false
model_reasoning_summary = "none"

[model_providers.routatic]
name = "routatic local proxy"
base_url = "http://127.0.0.1:3456/v1"
wire_api = "responses"
requires_openai_auth = false
supports_websockets = false
```

本地入口不需要将 CommandCode 密钥交给 Codex。不要使用已废弃的 `wire_api="chat"`，也不要将 Codex Base URL 直接指向只有 Chat Completions 的官方端点。配置字段依据 [Codex 自定义 provider 文档](https://developers.openai.com/codex/config-advanced)及当前配置参考。

已用本机 `codex-cli 0.144.3-cometix` 在隔离环境中验证：CLI 发起请求 → 分片 JSON 函数调用 → 执行合成 `printf` 工具 → 工具结果回传 → 最终文本；数据库产生两条正确归属的记录。这是实际 CLI 与合成上游的测试，不是 CommandCode 真实账户联调。

支持边界：

- 流式/非流式、文本、用户图片、instructions、JSON `function` 工具（`strict:false`）、字符串工具结果和缓存 usage。
- Codex 0.144 会把部分工具放进 `{"type":"namespace","name":"multi_agent_v1","tools":[…]}` 容器，与普通 function 工具并列发送。容器只是分组，适配器就地展开后再走同一套工具校验，因此其中的 custom/strict/无名工具仍被拒绝。2026-09-13 之前该容器被当作未知字段，导致所有 Codex 请求以 `json: unknown field "tools"` 失败。
- 不提供服务端会话/响应存储、`previous_response_id`、`conversation`、后台任务、非空 `include`、reasoning summary/加密 reasoning、托管搜索/执行工具、custom/freeform 工具、严格 JSON 输出或 WebSocket。
- 无法无损映射的请求返回明确的 HTTP 400；不是静默丢字段。客户端版本、模型能力设置或插件引入上述能力时，需要相应关闭或另行实现并测试，不能宣称支持所有 Codex 功能。
- `anthropic_first` 只作用于 Messages 入口，不接管 Codex Responses。

### 峰谷计费（2026-09-13 核实）

CommandCode 与 OpenCode Go 使用**同一套峰谷规则**：高峰为周一至周五的 01:00-04:00 与 06:00-10:00 UTC，其余（含周末）为 Off-Peak，倍率 2。依据 [GOAT 计划文档](https://commandcode.ai/docs/plans/goat)，每个受影响模型的行下方标注 `Off-peak shown (17h/day) · peak $X / $Y 01–04 & 06–10 UTC, Mon–Fri`。

覆盖的模型按该文档逐条列出，不按家族整体匹配——`deepseek/deepseek-v4-flash-fast` 由 API 提供但**不带**该标注，必须保持 Off-Peak：

| 模型 | Off-peak（输入/输出） | Peak |
| --- | --- | --- |
| `deepseek/deepseek-v4.1-flash` | $0.15 / $0.60 | $0.30 / $1.20 |
| `deepseek/deepseek-v4-flash` | $0.15 / $0.60 | $0.30 / $1.20 |
| `deepseek/deepseek-v4-flash-vision-exp` | $0.22 / $0.66 | $0.44 / $1.32 |
| `deepseek/deepseek-v4-pro` | $0.66 / $1.98 | $1.32 / $3.96 |

`commandcode.ai/models` 与 `/pricing` 都只显示 Off-Peak 单价（模型页以 `+1` 标注按模型的 deal），峰谷标注只出现在上述文档页；只查 Models 端点或价格页会得出「没有峰谷」的错误结论。

判定入口是 `history.ProviderPeakMultiplier`（`internal/history/record.go`），存储层、费用估算、面板与回填共用这一处；`internal/models.ModelFamily` 负责把 `deepseek/deepseek-v4-flash` 与 `deepseek-v4-flash` 归一到同一族名。

### 2026-09-13 隔离实例实测（Codex 工具往返 + 两个上游协议）

本机 Codex `0.144.3-cometix`、Claude Code `2.1.263` 经 SSH 隧道调用远端新二进制（commit `8133635`）的 loopback 隔离实例，独立配置与独立 DB；生产路由、生产服务和生产 DB 全程未改动。模型为 `deepseek/deepseek-v4-flash` 与 `moonshotai/Kimi-K2.6`。

| 客户端 | 别名（上游模型） | 结果 |
| --- | --- | --- |
| Claude Code（`ANTHROPIC_BASE_URL` 注入） | cc-deepseek（Chat Completions） | `CC_CLAUDE_OPENAI_OK`、`end_turn` |
| Claude Code（同上） | cc-kimi（Chat Completions） | `CC_CLAUDE_KIMI_OK` |
| Claude Code（Read 工具往返） | cc-deepseek | 读回 `CC_CLAUDE_TOOL_MARKER` |
| Codex（`env_key` 注入） | cc-deepseek | `CC_CODEX_OPENAI_OK`、`turn.completed` |
| Codex（shell 工具往返） | cc-deepseek | 模型调用 → Codex 执行 `printf CC_TOOL_OK` → 回传 → 最终文本 |
| Codex（shell 工具往返） | cc-kimi | 同上，`CC_KIMI_TOOL_OK` |

- 隔离 DB 全部记录 `provider=commandcode`、`success=1`。多轮请求的上游缓存读取量在 512–10112 之间，说明前缀缓存已在实际链路上产生命中；`cache_control` 现在确实进入上游报文（由 `TestModelFamilyRulesStillDiscriminate` 断言），但缓存命中同时也可能来自上游自动行为，两者未做因果分离。
- **Anthropic 上游分支无法在本账户上实测**：套餐不含 Claude 模型，`claude-sonnet-4-6` 与 `claude-haiku-4-5-20251001` 均返回 `403 MODEL_NOT_IN_PLAN`；强制非 Claude 模型走 Messages 端点则返回 `400 Model "deepseek/deepseek-v4-flash" is not supported on this endpoint. Use /provider/v1/chat/completions`。两条错误都是上游对请求的正确拒绝，说明端点选择与报文形状无误，但该分支的行为只由单元测试覆盖，没有真实端点证据。
- 生产部署后复验：服务 active、NRestarts=0、release `20260913200404-e8964f7be618`（commit `8133635`）、DB `quick_check=ok` 且 6561 条 `requests`/`provider_usage` 无丢失。
- 顺带发现（与本次改动无关）：生产 OpenCode Go 返回 `401 CreditsError: Insufficient balance`，且生产 `model_overrides` 中没有任何模型指向 CommandCode——该密钥已配置但未被路由使用。老 release 在部署前 80 分钟内服务了 0 个请求，因此这不是回归，而是上游计费状态。

## 日志、套餐和统计

### 2026-09-12 指定 DeepSeek 模型真实实测

本机 Codex `0.144.3-cometix`、Claude Code `2.1.263` 经 SSH 隧道调用远端已部署 `d7b25fb` 二进制的 loopback 隔离实例，上游固定为 `deepseek/deepseek-v4.1-flash`。没有改变正式默认路由、全局客户端配置或使用替代模型，测试记录位于独立数据库。

- Codex 主请求返回 `CC_CODEX_OK`、`turn.completed`。CLI 输入 7075（缓存 4352）、输出 6；DB 纯输入 2723、缓存读取 4352、输出 6，逐项一致。
- Claude 主请求返回 `CC_CLAUDE_OK`、`end_turn`、退出码 0。输入 144、输出 21、缓存 0，与 DB 一致。
- **兼容缺口已修复**：Claude 自动生成会话标题发送 `output_config.format`，原实现在 `internal/transformer/request.go` 主动拒绝该结构化输出（1 次流错误、3 次非流式 502）。现改为无损映射为 OpenAI `response_format`：`{"type":"json_schema","schema":S}` → `{"type":"json_schema","json_schema":{"name":"response","schema":S,"strict":true}}`。Anthropic 只定义 `json_schema` 一种变体，且要求 schema 闭合（`additionalProperties:false` 且属性全部必需），与 OpenAI strict 模式的要求一致，因此映射不丢失约束；未知的 `format.type` 仍然显式报错，不会静默丢弃。
- 两个测试库的费用均为未知；Claude CLI 对第三方模型显示 `costBasis: unknown`，其金额不作为官方扣费证据。测试进程、隧道和临时凭证副本已清理。

映射后的回归在 `internal/transformer/request_test.go`（`TestTransformRequestMapsStructuredOutputFormat`、`TestTransformRequestRejectsUnknownStructuredOutputFormat`）与本轮全量 `go test -race -p 2 ./... -count=1` 中通过。上游是否对每个模型都接受 `response_format` 仍无单独承诺：[官方 Provider 文档](https://commandcode.ai/docs/provider)继续要求非 Claude 模型使用 Chat Completions，并引用标准请求 schema，但没有逐模型保证严格 `json_schema` 能力。因此“映射正确”已实测，“上游逐模型支持”仍属未验证，不能仅凭“OpenAI 兼容”宣称支持。实测摘要见本地任务记录 `../.codex-tasks/20260910-platform-integration/tasks/14-commandcode-live/raw/live-result.json`（该目录不随仓库分发）。

### 日志与账户数据来源

- 五个平台的配置、路由健康、历史、性能和模型费用按平台归属；概览、历史、性能、分析可分别筛选平台，相同模型 ID 不再跨平台串账。套餐页另有平台独立的本实例请求、Token、费用及缺价统计，详见[五平台能力矩阵](platform-integration-review.md#五平台页面与账户能力)。
- 价格缺失时显示 `—`，混合已知/未知金额显示“已知”小计及未知记录数。平台账单、代理估算和未知费用不能互相替代；峰谷倍率只作用于其所属平台。
- CommandCode 账户数据使用经 Edge 页面和真实 API Key 核实的官方 Alpha 只读接口，不需要浏览器 Cookie。保留 [Usage](https://commandcode.ai/usage)、[Billing](https://commandcode.ai/billing) 和 [API Keys](https://commandcode.ai/settings/keys) 入口；Alpha 不是稳定公开合同，字段可能变化，不能套用 Go 配额。
- 官方 Provider API 文档当前说明除 Go 外的方案有 API 访问能力；账户的实际授权仍以上游为准。`401`/`403`/ZDR `422` 应检查密钥、套餐和模型能力，不通过私有 CLI 仿装规避。
- Analytics 日期/桶和概览“今日”是 UTC；历史日期筛选与单条时间是浏览器本地时区。日志刷新失败会显示错误，不能把未获取当作没有请求。
- `debug_capture` 仍是显式调试功能，包含完整对话内容；默认不要开启，不能把它当成脱敏的普通日志。

### CommandCode 账户查询

`GET /api/quota?provider=commandcode` 只使用独立 CommandCode Key，查询同一配置网关的三个端点：

| Alpha GET 端点 | 展示内容 |
| --- | --- |
| `/alpha/billing/credits` | 免费、月度剩余、购买点数；5 小时与每周窗口的已用/上限 |
| `/alpha/billing/subscriptions` | 套餐、状态、完整 UTC 周期与期末取消状态 |
| `/alpha/usage/summary` | 官方周期请求数、Token 与已消耗点数 |

使用 `Authorization: Bearer`；从 `commandcode.base_url` 的 `/provider/v1` 路径推导同源 Alpha 路径，保留网关前缀。不识别的自定义路径明确报错，不把密钥转发到猜测的官方域名。重定向不跟随，身份和支付字段不返回面板。

美元计价的点数不是现金余额，也不是本地费用；多 Key 分别展示、不合计。Alpha 没有 `monthlyCreditsGranted`，不由剩余值推算月度百分比；仅滚动窗口自身的 `used/cap` 可计算比例。`resetAt` 是 Unix 毫秒，0 显示未知，不显示 1970。

账户响应按端点和 Key 集合缓存 30 秒，`refresh=1` 手动刷新。额度、订阅、汇总各自保留错误；一个失败不会清空其他已成功数据。详细证据见本地任务记录 `../.codex-tasks/20260910-platform-integration/tasks/09-commandcode-account/raw/browser-contract.md`（该目录不随仓库分发）。

## 指定参考项目与验证范围

审阅了 [MAXeaglet/commandcode-proxy](https://github.com/MAXeaglet/commandcode-proxy/tree/487f219f9586b2a4ba7f7435eed7ee19dabc53ec)，固定提交 `487f219f9586b2a4ba7f7435eed7ee19dabc53ec`，MIT。它公开的入口是 Chat Completions、Messages、Models 和 health，没有 Responses 入口，因此不能直接填补 Codex 协议缺口。

本次参考其工具/流式/缓存行为检查项，实际实现复用本项目 Go 转换器；没有复制该项目源码、安装脚本、私有 CLI 生命周期调用或设备指纹逻辑，也不新增 Node 代理进程。仓库上游另行按语义移植，见 [修复与上游记录](platform-integration-review.md)。

已验证合成上游协议、隔离 Codex CLI、Go race/vet、六目标无 CGO 编译与 Chromium 三种宽度的面板交互。未验证真实账户扣费、所有模型和客户端版本、Firefox/Safari、Windows/Linux 实机运行或 macOS 原生托盘；编译通过不等于这些能力全部兼容。
