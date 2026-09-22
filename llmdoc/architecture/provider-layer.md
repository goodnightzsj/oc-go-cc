# Provider 层与平台边界

**关键区分：平台数 ≠ provider 实现数。** 平台身份的唯一所有者是 `internal/site/site.go:95` 的 `registry`（当前 6 个 descriptor 中 5 个已登记 provider，`openrouter` 无 registry 实现）——平台数、展示顺序、`RateTable` 归属全部以该表为准，本节不重述。加平台只需改这一张注册表。把 `internal/provider/` 读成"每个平台各一个文件"是错的。

## 注册与分派

| 环节 | 位置 |
|------|------|
| 共享 HTTP transport + 密钥轮询 | `internal/provider/provider.go:15` / `:23` / `:41`（`nextAPIKey`） |
| 登记点（权威） | `internal/server/server.go:71-75`，与 `site` registry 中实现齐全的平台一一对应 |
| 线程安全查找 | `internal/core/registry.go:22`（`Register`）/ `:34`（`Get`） |
| 运行期分派 | `internal/handlers/messages.go:837`（流式）/ `:1032`（非流式），均取 `providerRegistry.Get(client.Provider(model))` |
| provider 名来源 | `internal/client/opencode.go:188`（`Provider` → `config.NormalizeProvider`） |

Registry 未命中时**回落到 legacy client**（`internal/handlers/messages.go:900-902` 注释点明 OpenRouter 属此类），走 `h.client.GetStreamingBody` / `h.executeOpenAIRequest`。`internal/client/opencode.go` 因此仍是活跃的第二条代码路径：`getEndpoint`（`:208`）与 `ChatCompletion`（`:256`）。

配置层接受的名字由注册表决定：`SupportedProvider`（`internal/config/config.go:383`）内部转 `site.IsKnown`，凭证绑定 `ProviderAPIKeys`（`:434`）。

## Wire format

`internal/core/provider.go:14` 定义 `WireFormat`，常量在 `:16-24`：`WireFormatOpenAIChat`、`WireFormatAnthropic`、`WireFormatOpenAIResponses`、`WireFormatGemini`。`Provider` 接口在 `:49`（`Name`/`WireFormat`/`Execute`/`Stream`）；`ModelWireFormat`（`:67`）允许模型级显式覆盖 provider 推断。

各 provider 的推断方式不同，改动其一时不要假设其它相同：

| Provider | 推断依据 |
|----------|----------|
| OpenCode Go | `models.IsAnthropicModel`（`internal/provider/opencode_go.go:36`） |
| OpenCode Zen | `models.ClassifyEndpoint`（`internal/provider/opencode_zen.go:37`） |
| AWS Bedrock | 模型 ID 前缀（`internal/provider/aws_bedrock.go:38`） |
| CommandCode | `claude-*` → Anthropic，其余 Chat Completions（`internal/provider/commandcode.go:36`） |

CommandCode 入口：`internal/provider/commandcode.go:27`（`NewCommandCodeProvider`）/ `:43`（`Execute`）/ `:72`（`Stream`）/ `:76`（`request`）；`x-cmd-zdr` 头在 `:114`。加载器默认值 `internal/config/loader.go:35-36`；非 openai/anthropic 的 wire format 在 `loader.go:590` 被拒绝。

## 平台展示顺序

顺序只影响标签排序，不影响路由、密钥顺序或统计归属。

- 权威排序：`site.Order`（`internal/site/site.go:166`），排序值来自 registry 的 `Descriptor.Order`。`internal/config/provider_display.go:13` 的 `CompareProviderDisplay` 只是委托（`:10-12` 的注释说明它只排标签）。
- Go 侧调用点：`internal/handlers/models.go:57`、`internal/catalog/resolve.go:151`、`cmd/routatic-proxy/main.go:757`、`cmd/routatic-proxy/init_provider.go:77`。
- **JS 镜像必须同步**：`internal/gui/assets/app.js:2706`（`PROVIDERS`，只列可见平台）、`:2700`（`HIDDEN_PLATFORMS`，隐藏平台仍保留标签以便渲染历史行）、`:2721`（`PROVIDER_ORDER`，两者拼接成完整展示顺序）、`:2724`（`compareProviderDisplay`）。使用点 `:2283`、`:4074`、`:4389`、`:4945`。
- 行为守卫：`internal/gui/provider_display_behavior_test.go:5`，同时断言 fallback 链**不会**被重排。

## 缺口

- `internal/client/opencode.go` 作为 OpenRouter 及所有未登记 provider 的第二条路径，尚无独立文档页。
- `internal/provider/platform_protocol_test.go:50` 只遍历五个已登记名（`:49` 的 `TestProviderScopedHeaders`），不覆盖 OpenRouter 的分野。
