# 入站协议面

## 路由表

全部注册在 `internal/server/server.go:131-136`。

| 路径 | Handler |
|------|---------|
| `POST /v1/messages` | `handlers.NewAnthropicFirstHandler`（`internal/handlers/anthropic_first.go:134`）包装 `MessagesHandler.HandleMessages` |
| `POST /v1/responses` | `MessagesHandler.HandleResponses`（`internal/handlers/responses.go:19`） |
| `POST /v1/messages/count_tokens` | `HealthHandler.HandleCountTokens`（`internal/handlers/health.go:92`） |
| `GET /v1/models` | `ModelsHandler.HandleListModels`（`internal/handlers/models.go:49`） |
| `GET /health` | `HealthHandler.HandleHealth`（`internal/handlers/health.go:35`） |
| `GET /statusline` | `HealthHandler.HandleStatusline`（`internal/handlers/health.go:76`） |

## Responses 是适配器，不是第二套引擎

`responses.go:41` 调 `transformer.ResponsesToMessageRequest`，`:52-61` 把克隆后的请求重新指向 `/v1/messages` 并调用 `HandleMessages`；返回侧 `responsesHTTPWriter`（`responses.go:70`）把 Anthropic SSE 转回 Responses SSE。因此 Responses 入口的能力**上限就是 Messages 管线**，且是**无状态**子集。

## Fail-closed 边界

所有拒绝集中在一个函数：`internal/transformer/responses_inbound.go:94-293`（`ResponsesToMessageRequest`）。逐项拒绝（含行号）：

- 状态字段 `store` / `background` / `previous_response_id` / `conversation` / `include`（`:102-111`）
- `text.format` / `text.verbosity` 非纯文本（`:112-120`）
- 非正 `max_output_tokens`（`:123-125`）
- `reasoning.summary`（`:129-131`）；`reasoning.effort` 超出 `low|medium|high|max`（`:136-137`）
- 工具 `type != "function"`（`:141-143`）；函数工具非 `strict:false`（`:144-146`）
- `tool_choice` 非 auto/none/required/具名函数（`:177`）；具名函数未声明（`:199`）
- 不支持的角色（`:226`）；`system`/`developer` 出现在会话内容之后（`:235`）
- 重放带加密或内容的 `reasoning` 项（`:262-263`）；未知 input item 类型（`:273`）；非文本工具输出（`:255`）
- `input_image` 带 `file_id` 或 `detail != auto`（`:319-320`）；未知 content part 类型（`:340`）

支持面：字符串或数组 `input`；`message` / `function_call` / `function_call_output` / `reasoning(summary_text)` 项（`:219-274`）；纯文本与 http(s)/base64 `input_image`（`:316-338`）；JSON-object 函数工具（`:150-164`）；`instructions`/system 折入 `system`（`:216-217`、`:289-291`）。

## 出站同样 lossless-or-fail

`responses_inbound.go:408`（`AnthropicMessageToResponse`）、`:450`（`ResponsesStreamWriter`）、`:756`（`Finish`）：缺失 `message_stop` 视为失败响应，**从不伪造成功**。

## 相关文档

- `architecture/model-routing.md` — 进入管线后的模型选择
- `architecture/provider-layer.md` — Responses wire format 由哪个 provider 承接
