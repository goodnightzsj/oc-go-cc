# Provider 层与平台边界

平台身份以 `internal/site/site.go` 的 registry 为准；当前六平台均有运行期provider，在 `internal/server/server.go:69` 登记。新增平台仍须登记发送实现，描述符不自动创建它。

## 注册与分派

- `internal/provider/provider.go:15` 持有HTTP transport和密钥轮换；`config.ProviderAPIKeys` 是推理密钥池的唯一来源。
- `internal/handlers/messages.go:649`（流式）和 `:974`（非流式）只从registry分派；handler必须注入registry，目标未登记即显式失败。
- `internal/client/opencode.go` 仅保留timeout、provider身份、APIError和CaptureBody等共用支持，旧HTTP发送路径已删除。
- `internal/provider/openrouter.go:26` 创建OpenRouter发送器：所有模型固定Chat Completions与Bearer，保留 /v1 URL补全、归属头、推理密钥池的既有全局回退，Management Key不用于推理。显式非openai格式发送前报错；保留旧路径不应用低价值回复启发式的行为。
- `internal/router/fallback.go:198` 的AllowAttempt及 `:205` 的RecordAttempt为两个执行循环共享：单key认证失败阻断本请求内该平台，多key继续轮换，限流跳过平台，5xx更新同一熔断器；父context取消不惩罚上游，已发送真实SSE payload后不切模型。

## Wire format

`internal/core/provider.go:14` 定义 `WireFormat`，常量在 `:16-24`：`WireFormatOpenAIChat`、`WireFormatAnthropic`、`WireFormatOpenAIResponses`、`WireFormatGemini`。`Provider` 接口在 `:49`（`Name`/`WireFormat`/`Execute`/`Stream`）；`ModelWireFormat`（`:67`）允许模型级显式覆盖 provider 推断。

各 provider 的推断方式不同，改动其一时不要假设其它相同：

| Provider | 推断依据 |
|----------|----------|
| OpenCode Go | `models.IsAnthropicModel`（`internal/provider/opencode_go.go:36`） |
| OpenCode Zen | `models.ClassifyEndpoint`（`internal/provider/opencode_zen.go:37`） |
| AWS Bedrock | 模型 ID 前缀（`internal/provider/aws_bedrock.go:38`） |
| OpenRouter | 所有模型固定Chat Completions（`internal/provider/openrouter.go:32`） |
| CommandCode | `claude-*` → Anthropic，其余 Chat Completions（`internal/provider/commandcode.go:36`） |
| ClinePass | 所有模型固定 Chat Completions，上游始终 SSE，非流式本地聚合（`internal/provider/cline_pass.go:40`） |

CommandCode 入口：`internal/provider/commandcode.go:27`（`NewCommandCodeProvider`）/ `:43`（`Execute`）/ `:72`（`Stream`）/ `:76`（`request`）；`x-cmd-zdr` 头在 `:114`。加载器默认值 `internal/config/loader.go:35-36`；非 openai/anthropic 的 wire format 在 `loader.go:590` 被拒绝。

## ClinePass 渠道限制与观测

- 设置 `cline_pass.channel_pin_enabled` 默认关闭，`channel_pin` 保存网关 slug；空或纯空格在加载时实际默认 `deepseek`，不自动打开开关。非空非法 slug 仍拒绝保存，设置作用于所有 ClinePass 模型，热更新下一次请求生效（`internal/config/loader.go` 的 `applyDefaults`/`validate`）。
- 共享发送入口同时写入 `provider.only` 与 `providerOptions.gateway.only`；`z-ai`/`zai` 按两条管道分别拼写。关闭时两字段完全省略，不修改模型/订阅池前缀、密钥轮换或模型兜底（`internal/provider/cline_pass.go:80`）。
- `internal/provider/cline_pass_channel.go:18` 包装上游响应流，只观测根对象或 message/delta 中的 `provider_metadata.gateway.routing.finalProvider`；保留原字节与错误，不缓冲完整响应。元数据可能晚于 `finish_reason`，需读到 `[DONE]`。
- 日志含 `request_id`、`requested_provider`、`actual_provider`、`adherence`。完整且匹配为 `matched`，完整但不同为 `mismatch`，缺元数据、未完成或跳过超长行则 `unverified`。匹配不等于限制生效；不存在渠道仍正常完成才是限制被忽略的强反证。
- 标准 Messages/Responses 输出不新增渠道字段，CompactGate 转换后响应通常不能证明实际渠道；应关联代理观测日志。异常观测只告警，不丢弃输出或自动重试扣费。详细合同与实验见 [ClinePass 文档](../../docs/cline-pass.md)。
- GUI 候选由 `internal/gui/channel_catalog.go:88` 的snapshot及 `:113` 的channelLoop管理，通过 `GET /api/cline-pass/channels` 暴露：关闭capture始终使用嵌入JSON；开启后启动及每小时扫描现有capture增量，仅依据响应自身model/canonicalSlug归属提取actual/available渠道，不用request_id配对，不新增采集或改debug_capture。集合不变不写capture目录的channel-catalog.json；失败保留有效候选并公开安全错误状态。模板来自已核实的上游原始capture，非CompactGate转换后输出。纯headless不运行GUI扫描。
- `internal/config/validation_error.go` 提供字段/code和保留诊断的错误链；GUI通过 `internal/gui/settings_error.go:58` 输出固定中文与saved状态，不输出原始异常。主设置、降级链、导入与GUI开关共享反馈语义：设置失败保留草稿、定位可编辑字段；网络结果未知不自动重试，保存成功后的刷新失败不称为保存失败。自启动系统操作失败返回错误，不发布成功状态。
- 保存期间的新编辑不进入已保存基线、不被随后刷新覆盖；导入响应只控制自身预览，迟到结果不关闭新弹窗，旧原生close事件不删除新预览按钮。行为守卫为 `internal/gui/settings_behavior_test.go`；候选目录的隐私、轮转、小时更新与取消守卫为 `internal/gui/channel_catalog_test.go`。

## 平台展示顺序

顺序只影响标签排序，不影响路由、密钥顺序或统计归属。

- 权威排序：`site.Order`（`internal/site/site.go:166`），排序值来自 registry 的 `Descriptor.Order`。`internal/config/provider_display.go:13` 的 `CompareProviderDisplay` 只是委托（`:10-12` 的注释说明它只排标签）。
- Go 侧调用点：`internal/handlers/models.go:57`、`internal/catalog/resolve.go:151`、`cmd/routatic-proxy/main.go:757`、`cmd/routatic-proxy/init_provider.go:77`。
- **JS 镜像必须同步**：`internal/gui/assets/app.js:2706`（`PROVIDERS`，只列可见平台）、`:2700`（`HIDDEN_PLATFORMS`，隐藏平台仍保留标签以便渲染历史行）、`:2721`（`PROVIDER_ORDER`，两者拼接成完整展示顺序）、`:2724`（`compareProviderDisplay`）。使用点 `:2283`、`:4074`、`:4389`、`:4945`。
- 行为守卫：`internal/gui/provider_display_behavior_test.go:5`，同时断言 fallback 链**不会**被重排。

## 回归

- `internal/provider/openrouter_test.go`：URL、密钥轮换/全局回退、header隔离、缓存转换、取消和错误合同。
- `internal/server/openrouter_test.go`：生产登记点、Messages/Responses各两种模式、独立记账和capture完整性。
- `internal/handlers/stream_policy_test.go`：流式认证/熔断、跨执行模式共享健康状态、OpenRouter空完成回复兼容。
- `internal/config/cline_pass_channel_test.go`、`internal/gui/cline_pass_channel_test.go`：校验、持久化、开关热更新、目标保留及真实前端保存逻辑。
- `internal/provider/cline_pass_channel_test.go`：流式/非流式双字段、off→on→off、别名、晚到元数据、缺失/不匹配/超长行、错误透传与并发关闭。
