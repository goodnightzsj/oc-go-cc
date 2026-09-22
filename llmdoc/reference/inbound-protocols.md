# 入站协议面

## 路由表

全部注册在 `internal/server/server.go:135-140`。

| 路径 | Handler |
|------|---------|
| `POST /v1/messages` | `handlers.NewAnthropicFirstHandler`（`internal/handlers/anthropic_first.go:134`）包装 `MessagesHandler.HandleMessages` |
| `POST /v1/responses` | `MessagesHandler.HandleResponses`（`internal/handlers/responses.go:19`） |
| `POST /v1/messages/count_tokens` | `HealthHandler.HandleCountTokens`（`internal/handlers/health.go:92`） |
| `GET /v1/models` | `ModelsHandler.HandleListModels`（`internal/handlers/models.go:49`） |
| `GET /health` | `HealthHandler.HandleHealth`（`internal/handlers/health.go:35`） |
| `GET /statusline` | `HealthHandler.HandleStatusline`（`internal/handlers/health.go:76`） |

`/v1/chat/completions` **不在表内**：它只是出站 wire format，从未注册为入站路由。

## Responses 是适配器，不是第二套引擎

`responses.go:41` 调 `transformer.ResponsesToMessageRequest`，`:51-61` 把克隆后的请求重新指向 `/v1/messages` 并调用 `HandleMessages`；返回侧 `responsesHTTPWriter`（`responses.go:70`）把 Anthropic SSE 转回 Responses SSE。因此 Responses 入口的能力**上限就是 Messages 管线**，且是**无状态**子集。

## Fail-closed 边界

所有拒绝集中在一个函数：`internal/transformer/responses_inbound.go:129-332`（`ResponsesToMessageRequest`）。逐项拒绝（含行号）：

- 状态字段 `store` / `background` / `previous_response_id` / `conversation` / `include`（`:137-146`）
- `text.format` / `text.verbosity` 非纯文本（`:147-155`）
- 非正 `max_output_tokens`（`:157-162`）
- `reasoning.summary`（`:163-166`）；`reasoning.effort` 超出 `low|medium|high|max`（`:167-173`）
- 工具 `type != "function"`（`:179-182`）；函数工具非 `strict:false`（`:183-185`）
- `tool_choice` 非 auto/none/required/具名函数（`:224`）；具名函数未声明（`:238`）
- 不支持的角色（`:265`）；`system`/`developer` 出现在会话内容之后（`:274`）
- 重放带加密或内容的 `reasoning` 项（`:302`）；未知 input item 类型（`:312`）；非文本工具输出（`:294`）
- `input_image` 带 `file_id` 或 `detail != auto`（`:358-359`）；未知 content part 类型（`:379`）

支持面：字符串或数组 `input`；`message` / `function_call` / `function_call_output` / `reasoning(summary_text)` 项（`:258-313`）；纯文本与 http(s)/base64 `input_image`（`:352-377`）；JSON-object 函数工具（`:189-203`）；`instructions`/system 折入 `system`（`:255-256`、`:328-330`）。

## 出站同样 lossless-or-fail

`responses_inbound.go:447`（`AnthropicMessageToResponse`）、`:489`（`ResponsesStreamWriter` 类型，构造函数 `NewResponsesStreamWriter` 在 `:506`）、`:796`（`Finish`）：缺失 `message_stop` 视为失败响应，**从不伪造成功**。

## 相关文档

- `architecture/model-routing.md` — 进入管线后的模型选择
- `architecture/provider-layer.md` — Responses wire format 由哪个 provider 承接
