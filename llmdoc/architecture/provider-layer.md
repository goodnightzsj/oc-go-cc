# Provider 层与平台边界

**关键区分：五个平台存在于配置与显示层，运行期只有四个 `core.Provider` 实现。** 把 `internal/provider/` 读成"五个平台各一个文件"是错的。

## 注册与分派

| 环节 | 位置 |
|------|------|
| 共享 HTTP transport + 密钥轮询 | `internal/provider/provider.go:15` / `:23` / `:41`（`nextAPIKey`） |
| 登记点（权威） | `internal/server/server.go:67-71`，只登记 `opencode-go`、`opencode-zen`、`aws-bedrock`、`commandcode` |
| 线程安全查找 | `internal/core/registry.go:22`（`Register`）/ `:34`（`Get`） |
| 运行期分派 | `internal/handlers/messages.go:794`（流式）/ `:965`（非流式），均取 `providerRegistry.Get(client.Provider(model))` |
| provider 名来源 | `internal/client/opencode.go:256`（`Provider` → `config.NormalizeProvider`） |

Registry 未命中时**回落到 legacy client**（`internal/handlers/messages.go:852-855` 注释点明 OpenRouter 属此类），走 `h.client.GetStreamingBody` / `h.executeOpenAIRequest`。`internal/client/opencode.go` 因此仍是活跃的第二条代码路径：`getEndpoint`（`:299-305`）与 `ChatCompletion`（`:324`）。

配置层接受五个名字：`internal/config/config.go:349`（`SupportedProvider`）与 `:361`（`ProviderAPIKeys`）。

## Wire format

`internal/core/provider.go:14` 定义 `WireFormat`，常量在 `:16-24`：`WireFormatOpenAIChat`、`WireFormatAnthropic`、`WireFormatOpenAIResponses`、`WireFormatGemini`。`Provider` 接口在 `:49`（`Name`/`WireFormat`/`Execute`/`Stream`）；`ModelWireFormat`（`:67`）允许模型级显式覆盖 provider 推断。

各 provider 的推断方式不同，改动其一时不要假设其它相同：

| Provider | 推断依据 |
|----------|----------|
| OpenCode Go | `models.IsAnthropicModel`（`internal/provider/opencode_go.go:36`） |
| OpenCode Zen | `models.ClassifyEndpoint`（`internal/provider/opencode_zen.go:37`） |
| AWS Bedrock | 模型 ID 前缀（`internal/provider/aws_bedrock.go:35`） |
| CommandCode | `claude-*` → Anthropic，其余 Chat Completions（`internal/provider/commandcode.go:36`） |

CommandCode 入口：`internal/provider/commandcode.go:22` / `:27` / `:43`（`Execute`）/ `:72`（`Stream`）/ `:76`（`request`）；`x-cmd-zdr` 头在 `:113-115`。加载器默认值 `internal/config/loader.go:35-36`；非 openai/anthropic 的 wire format 在 `loader.go:600` 被拒绝。

## 平台展示顺序

顺序只影响标签排序，不影响路由、密钥顺序或统计归属。

- 权威排序表：`internal/config/provider_display.go:14`（`providerDisplayRank`：opencode-go 0、commandcode 1、opencode-zen 2、aws-bedrock 3、openrouter 4），比较器 `:10`，文件头注释 `:8-9` 明确"orders labels only"。
- Go 侧调用点：`internal/handlers/models.go:57`、`internal/catalog/resolve.go:151`、`cmd/routatic-proxy/main.go:735`、`cmd/routatic-proxy/init_provider.go:70`。
- **JS 镜像必须同步**：`internal/gui/assets/app.js:2219`（`PROVIDERS`）与 `:2228`（`compareProviderDisplay`），使用点 `:1830`、`:3272`、`:3582`、`:3989`。
- 行为守卫：`internal/gui/provider_display_behavior_test.go:5`，同时断言 fallback 链**不会**被重排。

## 缺口

- `internal/client/opencode.go` 作为 OpenRouter 及所有未登记 provider 的第二条路径，尚无独立文档页。
- `internal/provider/platform_protocol_test.go:50` 只遍历四个已登记名，不覆盖 CommandCode/OpenRouter 的分野。
