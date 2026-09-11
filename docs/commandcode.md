# CommandCode、Claude Code 与 Codex 接入

核实日期：2026-09-11。接入使用官方 Provider API，不读取或修改用户的全局客户端配置。

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
- 不提供服务端会话/响应存储、`previous_response_id`、`conversation`、后台任务、非空 `include`、reasoning summary/加密 reasoning、托管搜索/执行工具、custom/freeform 工具、严格 JSON 输出或 WebSocket。
- 无法无损映射的请求返回明确的 HTTP 400；不是静默丢字段。客户端版本、模型能力设置或插件引入上述能力时，需要相应关闭或另行实现并测试，不能宣称支持所有 Codex 功能。
- `anthropic_first` 只作用于 Messages 入口，不接管 Codex Responses。

## 日志、套餐和统计

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

账户响应按端点和 Key 集合缓存 30 秒，`refresh=1` 手动刷新。额度、订阅、汇总各自保留错误；一个失败不会清空其他已成功数据。详细证据见[协议核实记录](../.codex-tasks/20260910-platform-integration/tasks/09-commandcode-account/raw/browser-contract.md)。

## 指定参考项目与验证范围

审阅了 [MAXeaglet/commandcode-proxy](https://github.com/MAXeaglet/commandcode-proxy/tree/487f219f9586b2a4ba7f7435eed7ee19dabc53ec)，固定提交 `487f219f9586b2a4ba7f7435eed7ee19dabc53ec`，MIT。它公开的入口是 Chat Completions、Messages、Models 和 health，没有 Responses 入口，因此不能直接填补 Codex 协议缺口。

本次参考其工具/流式/缓存行为检查项，实际实现复用本项目 Go 转换器；没有复制该项目源码、安装脚本、私有 CLI 生命周期调用或设备指纹逻辑，也不新增 Node 代理进程。仓库上游另行按语义移植，见 [修复与上游记录](platform-integration-review.md)。

已验证合成上游协议、隔离 Codex CLI、Go race/vet、六目标无 CGO 编译与 Chromium 三种宽度的面板交互。未验证真实账户扣费、所有模型和客户端版本、Firefox/Safari、Windows/Linux 实机运行或 macOS 原生托盘；编译通过不等于这些能力全部兼容。
