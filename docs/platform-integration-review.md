# 多平台修复、上游融合与验收记录

本轮基线为 fork `684235d`，官方合同与上游再次核实于 2026-09-11。本文记录已确认问题及修复范围，不代表整个项目已不存在缺陷。CommandCode 原生支持判断、独立配置和两客户端示例见 [接入指南](commandcode.md)。

## 已修复的功能与数据问题

| 优先级 | 根因与影响 | 修复与回归证据 |
| --- | --- | --- |
| P1 | 平台账单同步将不完整快照当作全部历史，可能误删其他平台或未匹配请求；同名模型还可能跨平台对账 | 同步/对账限定平台，保留未匹配行，歧义与冲突显式报告，失败事务不返回虚假的写入数。[provider_scope_test.go](../internal/storage/provider_scope_test.go) |
| P1 | 平台专属密钥与全局密钥池优先级不一致，错误 provider/端点可导致密钥错发 | 专属凭证优先、CommandCode 无全局密钥回退；所有路由入口统一校验 provider 和协议，专属请求头不跨平台复用。[provider_validation_test.go](../internal/config/provider_validation_test.go)、[provider_integration_test.go](../internal/client/provider_integration_test.go) |
| P1 | 显式 `wire_format` 未贯通发送/转换，Responses 工具调用及流尾 usage 可能丢失 | 发送与解析统一协议；补足工具事件、终止 usage 和取消/异常流处理，保留两种缓存 token 拆分语义。[platform_protocol_test.go](../internal/provider/platform_protocol_test.go)、[responses_upstream_test.go](../internal/transformer/responses_upstream_test.go) |
| P2 | 熔断、去重、性能和费用按模型 ID 而非平台+模型归属，跨平台同名模型互相影响 | 使用平台+模型身份，性能样本与成功率分母一致，未知状态不冒充失败。[provider_identity_test.go](../internal/router/provider_identity_test.go)、[latency_test.go](../internal/storage/latency_test.go) |
| P2 | 缺价模型被当成免费，Go 峰谷/套餐规则泄漏到其他平台，本地估算被误认为账户额度 | 区分已知零、未知与已知小计；缺价不参与最低成本选择；Go 规则按平台限定，多密钥时不伪造分账户套餐用量。[platform_data_test.go](../internal/storage/platform_data_test.go)、[selector_pricing_test.go](../internal/router/selector_pricing_test.go)、[quota_scope_test.go](../internal/gui/quota_scope_test.go) |
| P2 | 日期按字符串排序/本地日历分桶，跨时区或夏令时可能错序/错桶；P95/P99 未正确排序取位 | 历史排序与索引按实际时刻，Analytics 日期/桶及概览今日边界用 UTC；分位数使用排序后的 nearest-rank。[requests_index_test.go](../internal/storage/requests_index_test.go)、[metrics_test.go](../internal/metrics/metrics_test.go) |
| P2 | 局部设置保存丢失同级字段，脱敏值被当作真实密钥；筛选旧响应覆盖新结果 | 合并、校验和写入保持一致，拒绝混合掩码密钥池；保留平台设置和环境变量引用；历史请求防止过期响应覆盖且失败可见。[config_update_test.go](../internal/gui/config_update_test.go) |
| P2 | 小于 1 美元的费用分布以 1 为分母，Top-N 列表忽略未展示数据 | 分母使用全部数据真实合计/最大值；零值不产生正条形，未知费用不伪装完整占比。[dashboard_behavior_test.go](../internal/gui/dashboard_behavior_test.go) |
| P2 | 非流式上游工具参数不是 JSON object 时仍标记为成功，与流式和入站重放合同不一致 | 在共用响应条目转换中拒绝 null/数组/字符串/数字参数，四项负向回归先失败后通过。[responses_inbound_test.go](../internal/transformer/responses_inbound_test.go) |
| P2 | 概览、性能、分析只有平台分组，没有独立查询；旧异步响应可能覆盖新平台 | 平台范围贯通汇总、比较、趋势和性能；切换立即清除旧值，失败不显示为零。[platform_scope_test.go](../internal/gui/platform_scope_test.go)、[platform_filter_behavior_test.go](../internal/gui/platform_filter_behavior_test.go) |
| P2 | CSV 每页重新读取筛选，中途切换平台会混合数据 | 一次导出固定完整查询；501 条两页异步回归验证平台、日期、搜索与排序均保持初始范围。[platform_filter_behavior_test.go](../internal/gui/platform_filter_behavior_test.go) |
| P1 | 额度请求跟随重定向，错误响应体可能回显 Key | 拒绝重定向及含凭证/查询参数的额度地址；只返回脱敏错误。OpenRouter 管理 Key 与推理池隔离，逐 Key 保留失败。[openrouter_test.go](../internal/quota/openrouter_test.go)、[platform_quota_test.go](../internal/gui/platform_quota_test.go) |
| P1 | AWS 账单缺少可执行接入；收费查询若用 GET 会被预取或跨站触发 | 独立 IAM 配置和账户过滤，复用官方 SDK 签名/分页；手动 POST 与来源检查，自动刷新不查 AWS，缺币种或分页失败不展示部分合计。[bedrock_billing_test.go](../internal/gui/bedrock_billing_test.go)、[bedrock_test.go](../internal/quota/bedrock_test.go) |

普通日志不等于原始流量捕获：`debug_capture` 含完整对话内容，仍只应显式用于调试。CommandCode 已实现官方 Alpha 账户只读查询；数据源来自获授权的 Edge 协议核实，不导出网页 Cookie，不借用 Go 额度。各账户块的失败与未知值仍显式展示。

## 五平台页面与账户能力

| 平台 | 独立配置、日志、概览、性能、分析 | 套餐页本地数据 | 上游账户数据及剩余缺口 |
| --- | --- | --- | --- |
| OpenCode Go | 平台筛选及同名模型隔离 | 本实例请求、Token、已知费用与缺价数 | 保留现有 usage 窗口；端点未公开稳定合同，分模型费用不是官方账户账单 |
| OpenCode Zen | 同上 | 同上 | 未找到公开账户查询合同；仍未接入余额，保留官方控制台入口 |
| AWS Bedrock | 同上 | 同上 | 已实现 Cost Explorer 官方服务费用；独立 IAM 身份、显式账户、默认关闭、手动收费查询。保留币种/日期/Estimated，不冒充余额；真实 IAM 授权待验收。[配置与范围](aws-bedrock-billing.md) |
| OpenRouter | 同上 | 同上 | 已实现 `/key` 每 Key 限额/UTC 用量与独立 Management Key `/credits`；BYOK、Key 限额和账户余额分开，不合计多 Key |
| CommandCode | 同上 | 同上 | 官方 Alpha 额度、订阅、周期汇总，独立 API Key；美元计价点数不是现金，缺月度总额不推算百分比；Alpha 字段尚非稳定公开合同。[端点与边界](commandcode.md#commandcode-账户查询) |

平台状态明确区分已获取、部分失败、未配置、未提供能力和错误；官方数据与本地账本独立加载。只有经过本实例的请求才进入本地统计，不代表账户的全部消费。OpenRouter 管理 Key 必须单独设置 `openrouter.management_api_key` 或 `ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY`，不会使用推理 Key 代替，也不推断其与其它 Key 属于同一账户。

一手依据：[OpenRouter Key 用量](https://openrouter.ai/docs/api_reference/limits)、[Credits 的 Management Key 要求](https://openrouter.ai/docs/api/api-reference/credits/get-remaining-credits)、[Zen](https://opencode.ai/docs/zen/)、[CommandCode Usage Limits](https://commandcode.ai/docs/resources/usage-limits)、[AWS Cost Explorer 权限](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-api.html)。缺少公开合同或账户权限的部分仍是缺口，不能将本地账本、入口链接或合成测试描述为真实账户接入成功。

## 上游提交筛选

上游为 [samueltuyizere/oc-go-cc](https://github.com/samueltuyizere/oc-go-cc)，第一轮筛选截至 `b214eeb279d9a397872bbc0795c2486e3a0dd969` 的 27 项提交。2026-09-11 再核实 HEAD 为 `1f15a76c4dcb18db938714a28ae93047cbcc4f3e`；本轮按行为移植，不整树 merge。提交、推送与部署状态见[当前任务记录](../.codex-tasks/20260910-platform-integration/PROGRESS.md)。

| 提交 | 本轮处理 |
| --- | --- |
| `b82c865` | 采纳全部模型/家族覆盖的 provider 校验；未混入 updater 改造 |
| `e181e0a` | 采纳 Go Responses 端点及显式 `wire_format`，并统一相关平台发送/解析 |
| `ee74c4a` | 采纳 Responses 函数工具调用与输入转换修复 |
| `722ff60` | 采纳流尾 usage 记账，保留 fork 的缓存拆分 |
| `eab63d8` | 采纳平台专属请求头，避免头部错发 |
| `744895c` | 采纳 Go 的 Claude 会话 ID 转发，不发送给其他平台 |
| `7cf5012` | 采纳 Zen Responses/Gemini 模型前缀分类 |
| `bc6cda7` | 采纳 macOS Homebrew 稳定自启路径，避免升级后 Cellar 旧路径失效 |
| `478ec00` | README 使用实际存在的 `start`/`start -b`，移除旧 `ui` 示例 |
| `0b723b2` | 审阅 thinking 流结束与模型标识增量，保留 fork 已有等效修复，不重复覆盖 |
| `b64f155` | 仅采纳 P95/P99 正确性修复，不引入可能丢失账目的异步队列和未测性能重构 |

未采纳 `56d1f43` 的 RPM/流水线/多模块迁移、5 项 Go 依赖升级和 10 项 CI 依赖升级；它们不是当前缺陷的必要修复，需要各自验证，不能覆盖 fork 的发布流程。原有依赖版本未升级，`github.com/google/uuid` 仅从间接依赖改为直接依赖；AWS 账单增量新增官方 Go SDK config/costexplorer 及其依赖，不自行实现凭证链或 SigV4。

新增 [1f15a76](https://github.com/samueltuyizere/oc-go-cc/commit/1f15a76c4dcb18db938714a28ae93047cbcc4f3e) 不仅升级 setup-go v5→v7，还包含固定工具链后的 nfpm 版本回退及上游开发机的 Trunk 绝对路径。当前分支没有该 RPM 包装流程；不移植开发机路径，也不在未单独验证发布 CI 的情况下升级 action 的工具链语义。此增量不含本次账户或页面逻辑修复，留作独立 CI 升级，前述已采纳的功能修复保留。

用户指定的 [MAXeaglet/commandcode-proxy](https://github.com/MAXeaglet/commandcode-proxy/tree/487f219f9586b2a4ba7f7435eed7ee19dabc53ec)（MIT，固定 `487f219`）作为协议行为审阅参考；未复制源码或 CLI 设备指纹/生命周期调用。它没有 Responses 入口，本项目复用 Go 转换器实现必要的 Codex 适配。

## 验证与兼容边界

- 自动测试使用临时 HOME/配置/数据库和合成 HTTP 上游。浏览器 fixture 清除继承的平台环境覆盖项，浏览器隔离 context 阻断外网与系统自启写接口。
- 2026-09-10 阶段 race 通过 625 个顶层测试（含子测试 1000 项），vet 和六目标无 CGO 编译通过，Codex 冒烟显式通过；这不是新增平台筛选后的最终结果。当前源码验证及原始证据见 [任务验收记录](../.codex-tasks/20260910-platform-integration/PROGRESS.md)。
- 旧 Chromium 三宽度七页签结果只作为基线；本轮使用真实 Go/SQLite 合成后端重新验证五平台筛选、局部保存和账户错误状态，不能仅凭选项存在或旧模拟数据宣称兼容。
- Codex `0.144.3-cometix` 的隔离 CLI→合成上游双轮工具调用已验证，不等于真实 CommandCode 账户联调。Claude 原生接口的高级扩展仍取决于实际平台是否接受。
- 五个平台均能被配置并在本地数据中独立展示，不等于五个平台都提供同样的账单/余额 API。真实账户权限、生成/扣费、所有模型、所有客户端版本、Safari/Firefox、Windows/Linux 实机和原生托盘仍未覆盖，不能宣称“所有平台完全兼容”。
