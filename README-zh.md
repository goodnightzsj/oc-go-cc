# routatic-proxy [加入 Discord](https://discord.gg/pUrfwfTFxM)

[![Go Version](https://img.shields.io/github/go-mod/go-version/routatic/proxy)](https://go.dev/)
[![License](https://img.shields.io/github/license/routatic/proxy)](./LICENSE)

[English](./README.md) | **中文**

一个 Go CLI 代理，将 [Claude Code](https://code.claude.com/docs/en/llm-gateway-connect) Messages 和 [Codex](https://developers.openai.com/codex/config-advanced) Responses 请求路由到 OpenCode Go、OpenCode Zen、AWS Bedrock、OpenRouter 和 CommandCode，提供模型路由、日志和用量统计。

上游支持原生 Anthropic 时直接转发；其他链路按配置转换协议。Codex 入口支持有明确边界的无状态 Responses 子集，配置方式与限制见 [CommandCode 接入指南](docs/commandcode.md)。

Responses 原生目标保留严格工具、公开摘要、扩展字段及原始 JSON/SSE；失败仍按失败记账。加密重放限定精确模型、单密钥且禁止 fallback，不开放远端会话或已有文件/容器引用。调用方必须保持同来源，配置换源后开始新会话。

---

## macOS GUI 版本

本仓库为 `routatic-proxy` 额外提供了 macOS 原生图形界面支持（系统托盘 + 内嵌控制台面板）。

### 功能特点

历史请求主模型直接显示上游声明的 `response_model`（如 `vmc/...`），渠道徽标直接显示上游报告值（如 `private`），接入平台仍显示 ClinePass 等原平台；规范模型 `deepseek/...` 不再覆盖响应声明，放在详情中与路由/计费模型分别列出。搜索、模型排序和 CSV 同步保留这些信息；旧历史未记录声明时沿用已有上游/路由模型，不猜测回填，不改变下游模型选择或计费。上游自报不等于独立验证底层模型身份。

- **系统托盘图标** — 直接在 macOS 顶部状态栏中快捷控制代理服务的启动、停止、开机自启和退出。
- **交互式控制台** — 原生窗口控制台，支持查看实时历史请求、模型调用分布，并且无需手动编辑 JSON 配置文件，即可直接在界面中修改和保存 API Key。
- **DMG 一键安装包** — 提供标准的 macOS 应用程序打包，带有关机自启与双击运行托盘支持。

### 如何运行

您可以直接在此仓库的 **Releases** 页面下载编译好的 `.dmg` 安装包，或者在终端运行以下命令启动：

```bash
# 启动代理和浏览器面板
routatic-proxy start
# 后台运行
routatic-proxy start -b
```

浏览器访问 `http://127.0.0.1:3445`。Web 面板不依赖 CGO；原生托盘仅在 macOS + CGO 构建中提供。仅需要代理时使用 `serve`。

### 控制台预览

**仪表盘标签页：** 概览、历史请求、性能、降级策略、用量分析、用量与账单、设置。

概览、历史、性能和用量分析均可独立筛选平台；趋势、模型明细、汇总和比较使用相同平台范围。历史 CSV 导出固定开始时的筛选条件，不受分页期间切换平台影响。

历史、用量分析和性能聚合跟随 HTTP 请求取消，包括等待共享查询门或数据库连接；原有查询超时与缓存失效规则不变。OpenCode Go 和 CommandCode 的本地账本查询被取消时，不写入账户缓存。

同筛选的慢轮询复用在途请求，换筛选才取消旧请求；清空视图同时清空图表数据，缩放不会重画旧结果。请求结果与 token 观测独立：缺失用量保持未知，不显示为免费，明确报告的零仍是零。

协议转换对坏帧和缺失终态显式失败，不伪造完成，并保留失败前已报告的用量。公开思考和拒绝文本保留；工具结果图片可经 Responses 目标发送，其他无法表达的桥明确拒绝。Gemini 的 system、角色、图片、函数调用和控制字段保留；不透明 thoughtSignature 与 Responses 加密重放仍需绑定来源的原生路径。本地协议测试不证明模型质量等同官方渠道。

Chat Completions 与原生 Responses 上游报告的推理 Token 均保留到 Messages/Responses 的 JSON、流式输出及请求历史，失败时保留已报告的部分用量。Responses 使用 `usage.output_tokens_details.reasoning_tokens`，Messages 使用非标准扩展 `usage.reasoning_tokens`。推理 Token 已包含在输出总量中，不再次相加；转换输出省略未知或零值拆分，原生 Responses 保留源字段原样。OpenCode Go 的三种上游协议在捕获开启或关闭时均记录脱敏 key 标记。这些修复仅作用于新请求，不回填历史。

主列仅名 **Tok/s**，使用源流最终 output / 首个非空文本、思考或工具输出至成功源终态的耗时。观察点在预读、capture 和转换之前，排除首输出等待和 HTTP 收尾。详情显示“生成 Tok/s”及整个请求的“平均吞吐”；源样本缺失时不拿平均值顶替。

历史 API 提供可空 `generation_sample`（first_output_ms、duration_ms、output_tokens）及 `generation_tokens_per_second`，CSV 追加对应字段。小于 100ms 保留原始样本但不计算速率；零 output 是有效 0，失败、incomplete 或缺失样本显示 `—`。模型/平台按同批合格样本的总 output / 总时长计算，提供 `generation_samples`。旧行、非流式、Gemini 与 Anthropic-first 直通不新增样本。旧 first_event/content/message_end 及派生速率仍保留应用输出语义；旧 tokens_per_second API/排序和评分仍是请求平均值，主列排序改用 generation_tokens_per_second。推理已包含在 output，观测速率不是纯模型解码速度；WorkBuddy 网关平均值仍独立。

**概览** 给出请求、Token、成功率与费用，并在每张统计卡主值下附带可读的速率（`均 71.5K/次`、`12/分`）。平台健康表按平台给出成功率、平均延迟、实测吞吐与降级占比，断路状态以圆点标注。每日费用以柱状图呈现：区间内只有一天时收窄为单柱并在说明中点出该日金额，日期标签按可用宽度自动稀疏，避免重叠。

**历史请求** 支持按平台、状态、时间、模型、使用场景、请求类型和费用来源筛选，并按任意列排序。Token 列第一行是总数，第二行是提示 / 输出的拆分，不打开弹窗即可看出单次请求的形态。**Tok/s** 列显示并按源流生成速率排序，缺失样本显示破折号，两个方向都排在可测量行之后。被路由到其他模型时，详情弹窗在服务模型下方标出客户端**请求的模型**。命中平台峰时费率的记录带 `高峰 ×N` 角标——角标显示实际套用的倍率，因此在 DeepSeek 计价的平台上读作 `×2`，在 OpenRouter 的 `tencent/hy3` 上读作 `×1.6`——记录的费用按该倍率计算。在按量计费的平台上，记录的费用是该次用量的估算金额；在 ClinePass 这类包月订阅上根本没有按 token 收费——这个数是用来衡量套餐配额消耗的参考费率估算。峰谷适用于各平台自己发布了两档费率的模型：OpenCode Go、CommandCode、ClinePass 计价的 DeepSeek 各行，窗口为三家一致的 UTC 01-04 / 06-10（北京 09:00-12:00 / 14:00-18:00）工作日；以及 OpenRouter 三个 `pricing.overrides` 带时间窗的模型。中国法定节假日按规则排除：OpenCode Go 与 ClinePass 沿用 DeepSeek 的表述，CommandCode 自己写明规则且无此豁免、节假日照计高峰；OpenRouter 的窗口数据里没有节假日概念，同样照计高峰。日历取自国务院公告（每日抓取），并内嵌一份种子到二进制里，因此节假日按低谷计费而不是高峰。日志可通过搜索框按请求模型、服务模型、供应商、场景和错误信息检索。

**用量分析** 提供 KPI、按周期与按模型的明细表、Token 趋势和平台分布。模型表只展示一个源流生成 **Tok/s**；样本数与请求平均吞吐在提示中。可自定义起止日期（最长 92 天，支持小时粒度）。模型分布只列前 5 名并在标题标出 `前 5 / 19`，平台分布全部列出。

**价格来源** 面板说明每个平台的价格表来自实时抓取还是构建时快照，以及最近一次抓取时间。两者算出的费用在界面上完全一致，因此必须显式区分：本项目自身就遇到过内置快照数周内失效的情况（OpenCode Go 调整了 DeepSeek 价格、CommandCode 重新定价了四个模型），期间面板一直用已作废的费率显示着格式工整的数字。

**用量与账单** 为各平台分别展示本实例的请求、Token、费用已知小计和缺价记录。账户数据另行查询：OpenCode Go 保留三个限额窗口（5 小时滚动 / 本周 / 本月）；OpenRouter 查询每个推理 Key 的额度与 UTC 用量，并使用独立的 `openrouter.management_api_key` 查询账户 Credits，BYOK 用量单独显示。CommandCode 使用自己的 Key 查询官方 Alpha 点数、订阅与用量汇总，无需浏览器 Cookie。ClinePass 使用自己的 Key 读取套餐窗口，按参考费率折算为百分比而非应付金额。密钥仅以掩码展示；上述账户结果按平台缓存 30 秒。

AWS Bedrock 已接入 [Cost Explorer 独立账单查询](docs/aws-bedrock-billing.md)：默认关闭，需显式账户 ID 和服务进程的独立 AWS SDK 身份；仅手动按钮发起收费查询，普通刷新只读取同账户、同 UTC 日的缓存。展示最近 30 个完整 UTC 日内已列明服务的费用、币种与预估标记，不当作剩余额度。

Go 模型额度来自 [Go 文档](https://opencode.ai/docs/zh-cn/go)，分模型用量是本实例估算，不是官方账户账单；仅在配置单个 Go 密钥且月度窗口已知时展示，多密钥无法从本地记录可靠归属到账户。Go 用量端点与 CommandCode Alpha 端点尚非稳定公开合同；Zen 仍未取得账户查询合同。CommandCode 点数不是现金，接口未返回月度总额时不推算月百分比。详情见[五平台能力矩阵](docs/platform-integration-review.md#五平台页面与账户能力)。

未知费用显示 `—`，部分小计的值只显示 `$xx`，覆盖率和未知条数放独立说明，不给数值加“已知”前缀。当前窗口、今天和保留期分别计算覆盖率。用量分析的日期/时间桶和概览“今日”边界使用 UTC；历史日期筛选和单条时间使用浏览器本地时区。设置按平台折叠定位，排序和降级链调整支持键盘，并保留浅深主题及手机布局。

以下截图使用固定的合成演示数据，不包含生产请求、账户数据或凭证。可用 `python3 scripts/make-mock-data.py` 重新生成同数据集并复现这些截图。

#### 概览

![包含请求趋势、Token 趋势与平台健康表的仪表盘概览](assets/zh/dashboard-overview.png)

#### 历史请求

![包含筛选、分布统计与高峰计费角标的分页历史请求](assets/zh/dashboard-history.png)

#### 请求详情

![标出客户端请求模型与实际服务模型的请求详情弹窗](assets/zh/dashboard-request-detail.png)

#### 用量分析

![包含吞吐列、周期明细与平台分布的用量分析](assets/zh/dashboard-analytics.png)

#### 价格来源

![说明各平台价格表来自实时抓取还是构建时快照](assets/zh/dashboard-price-sources.png)

<details>
<summary>用量与账单、性能、降级策略和设置</summary>

![用量与账单页面](assets/zh/dashboard-quota.png)

![模型性能页面](assets/zh/dashboard-performance.png)

![降级策略编辑器](assets/zh/dashboard-fallback.png)

![代理设置页面](assets/zh/dashboard-settings.png)

</details>

---

## 为什么选择 routatic-proxy？

多个平台可以共用客户端入口、路由和本地观测。平台凭证分别配置；实际模型权限、套餐与费用以相应平台为准，原生支持和协议适配不代表客户端的所有高级功能都可用。

## 功能特性

- **多提供商支持** — 从单一配置路由到 OpenCode Go、OpenCode Zen、AWS Bedrock、OpenRouter 或 CommandCode
- **透明代理** — Claude Code 发送 Anthropic 格式请求，代理转换为目标提供商格式并返回
- **模型路由** — 根据上下文自动路由到不同模型（默认、思考、长上下文、后台）
- **流式场景路由** — 可配置的流式请求路由；为 Claude Code 多代理和审查工作流启用正确的场景选择
- **降级链** — 如果模型失败，自动尝试降级链中的下一个
- **熔断器** — 跟踪模型健康状况，跳过故障模型以避免延迟峰值
- **实时流式** — 完整的 SSE 流式传输，实时格式转换
- **工具调用** — 正确的 Anthropic tool_use/tool_result <-> OpenAI/Gemini 函数调用转换
- **Token 计数** — 使用 tiktoken (cl100k_base) 进行准确的 token 计数和上下文阈值检测
- **JSON 配置** — 灵活的配置文件，支持环境变量覆盖和 `${VAR}` 插值
- **热重载** — 监视配置文件变化并自动重新加载（默认关闭）
- **后台模式** — 作为守护进程运行，与终端分离
- **登录自启动** — 通过 launchd 在系统启动时启动（macOS）
- **自更新** — 一键检查并安装最新版本

## 支持的模型

### OpenCode Go 模型

| 模型 | 上下文 | 最佳用途 |
|------|--------|----------|
| **GLM-5.2** | ~200K tokens | 关键架构决策、生产代码审查 |
| **Kimi K2.7 Code** | ~256K tokens | 大型代码生成，32K 最大输出 |
| **Qwen3.7 Plus** | ~128K tokens | 通用编码，比 Qwen3.6 质量更好 |
| **Qwen3.7 Max** | ~128K tokens | 复杂编码，Qwen 最佳质量 |

完整模型列表（包括成本和路由建议）请参见 [MODELS.md](MODELS.md)。

### OpenCode Zen 模型

Zen 提供按使用量付费的额外模型：

- **Claude 模型**: Claude Fable 5, Claude Opus 4.8/4.6/4.5/4.1, Claude Sonnet 4
- **Gemini 模型**: Gemini 3.5 Flash, Gemini 3.1 Pro, Gemini 3 Flash
- **GPT 模型**: GPT 5.5, GPT 5.4, GPT 5.3 Codex 等
- **免费层**: DeepSeek V4 Pro, Grok Build 0.1, Big Pickle 等

完整的 Zen 模型列表请参见 [MODELS.md](MODELS.md#opencodes-zen)。

## 快速开始

### 1. 安装

```bash
# macOS / Linux
brew tap routatic/tap && brew install routatic-proxy

# Windows
scoop bucket add routatic https://github.com/routatic/scoop-bucket && scoop install routatic-proxy

# Docker（使用 Makefile）
cp .env.example .env                    # 然后在 .env 中填入你的 API key
make docker-up

# Docker（手动）
cp .env.example .env
docker build -t routatic-proxy .
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 routatic-proxy

# Docker 从 GitHub Container Registry
docker pull ghcr.io/routatic/proxy:latest
docker run -d --restart unless-stopped --name routatic-proxy --env-file .env -p 3456:3456 ghcr.io/routatic/proxy:latest
```

更多安装选项请参见 [docs/zh/INSTALLATION.md](docs/zh/INSTALLATION.md)。

### 2. 初始化配置

```bash
routatic-proxy init
```

在 `~/.config/routatic-proxy/config.json` 创建默认配置。编辑它以添加你的 API key，或设置环境变量：

```bash
export ROUTATIC_PROXY_API_KEY=sk-opencode-your-key-here
```

CommandCode 新配置可使用 `routatic-proxy init --provider commandcode`；已有配置请在设置页增加独立平台字段与模型映射，`init` 不会覆盖现有文件。完整步骤与 Codex 配置见 [接入指南](docs/commandcode.md)。

### 3. 启动代理

```bash
routatic-proxy serve
```

停止 Docker 容器（如果使用 Docker）：

```bash
make docker-stop
```

### 4. 配置 Claude Code

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
```

### 5. 运行 Claude Code

```bash
claude
```

## CLI 命令

```
routatic-proxy serve              启动代理服务器
routatic-proxy start              启动代理和面板（http://127.0.0.1:3445）
routatic-proxy start -b           后台启动代理和面板
routatic-proxy serve -b           后台启动（与终端分离）
routatic-proxy serve --port 8080  在自定义端口启动
routatic-proxy stop               停止运行中的代理服务器
routatic-proxy status             检查代理是否运行
routatic-proxy init               创建默认配置文件
routatic-proxy validate           验证配置文件
routatic-proxy models             列出模型目录中的模型
routatic-proxy autostart enable   启用登录自启动
routatic-proxy autostart disable  禁用登录自启动
routatic-proxy autostart status   检查自启动状态
routatic-proxy update             更新到最新版本
routatic-proxy update --check     检查是否有可用更新
routatic-proxy update --yes       静默更新，无需确认
routatic-proxy --version          显示版本
```

## 文档

| 文档 | 描述 |
|------|------|
| [docs/zh/INSTALLATION.md](docs/zh/INSTALLATION.md) | 安装指南 - Homebrew、Scoop、从源码构建、发布二进制 |
| [docs/zh/CONFIGURATION.md](docs/zh/CONFIGURATION.md) | 配置指南 - 配置文件参考、环境变量、模型路由、降级链 |
| [docs/commandcode.md](docs/commandcode.md) | CommandCode 独立配置、Claude Code/Codex 接入与兼容边界 |
| [docs/platform-integration-review.md](docs/platform-integration-review.md) | 功能修复、上游采纳记录与验证范围 |
| [docs/zh/MODELS.md](docs/zh/MODELS.md) | 模型指南 - 模型能力、成本和路由建议 |
| [CONTRIBUTING.md](CONTRIBUTING.md) | 贡献指南 - 开发环境、架构概览和如何提交 PR |
| [docs/zh/TROUBLESHOOTING.md](docs/zh/TROUBLESHOOTING.md) | 故障排除 - 常见问题和调试模式 |
| [docs/architecture.md](docs/architecture.md) | 系统设计 - 请求流程、模块概览 |
| [docs/reference-api.md](docs/reference-api.md) | HTTP API 参考（端点、流式、错误处理） |
| [docs/howto-add-model.md](docs/howto-add-model.md) | 添加模型 - 零代码变更 |
| [docs/howto-custom-routing.md](docs/howto-custom-routing.md) | 自定义路由 - 场景检测和模型选择 |
| [docs/howto-debug-routing.md](docs/howto-debug-routing.md) | 路由调试 - 常见问题和排查方法 |

## 贡献

欢迎贡献！请参阅 [CONTRIBUTING.md](CONTRIBUTING.md) 了解开发环境设置、架构概览以及如何提交 pull request。

## 致谢

- 套餐额度窗口的解析逻辑移植自 [ocusage](https://github.com/muzimu217/ocusage)（MIT）—— 该项目梳理了 OpenCode Go 未公开用量端点及其在真实环境中的多种响应形态。

## 许可证

[AGPL-3.0](LICENSE)
