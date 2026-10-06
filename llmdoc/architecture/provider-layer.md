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

- 设置 `cline_pass.channel_pin_enabled` 默认关闭，`channel_pin` 保存网关 slug；开启但目标为空会拒绝保存。设置作用于所有 ClinePass 模型，热更新下一次请求生效（`internal/config/config.go:286`、`internal/config/loader.go:417`）。
- 共享发送入口同时写入 `provider.only` 与 `providerOptions.gateway.only`；`z-ai`/`zai` 按两条管道分别拼写。关闭时两字段完全省略，不修改模型/订阅池前缀、密钥轮换或模型兜底（`internal/provider/cline_pass.go:80`）。
- `internal/provider/cline_pass_channel.go:18` 包装上游响应流，只观测根对象或 message/delta 中的 `provider_metadata.gateway.routing.finalProvider`；保留原字节与错误，不缓冲完整响应。元数据可能晚于 `finish_reason`，需读到 `[DONE]`。
- 日志含 `request_id`、`requested_provider`、`actual_provider`、`adherence`。完整且匹配为 `matched`，完整但不同为 `mismatch`，缺元数据、未完成或跳过超长行则 `unverified`。匹配不等于限制生效；不存在渠道仍正常完成才是限制被忽略的强反证。
- 标准 Messages/Responses 输出不新增渠道字段，CompactGate 转换后响应通常不能证明实际渠道；应关联代理观测日志。异常观测只告警，不丢弃输出或自动重试扣费。详细合同与实验见 [ClinePass 文档](../../docs/cline-pass.md)。

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
