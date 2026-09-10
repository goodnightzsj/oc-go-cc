# 协议终审记录

- 官方合同：CommandCode Provider API 的 Messages/Chat/Models；Codex 当前 `wire_api=responses`；第三方参考仓库无 Responses 入口。来源和选择理由见 official-support.md。
- 主线程直接复核：`internal/provider/commandcode.go`、`anthropic_payload.go`、`internal/client/headers.go`、`internal/core/request_metadata.go`、`internal/handlers/responses.go`、`internal/transformer/responses_inbound.go` 及其测试；追踪 `MessagesHandler.admitRequest`、RequestMetadata 设置及 `Finish` 的错误记账调用。
- 已检查不变量：CommandCode 只用专属 key 且禁止重定向携带 key；原生 Claude 保留原始字段；Responses 内部适配不重复限流；流式增量转发；共享路由/账本；不支持的服务端状态/托管工具/签名 reasoning 明确失败。

## P2：非流式工具参数错误被标作成功（已修）

- 最小输入：上游 Messages 响应含 `tool_use`，其 `input` 分别为 `null`、`[]`、字符串或数字。
- 修前：`AnthropicMessageToResponse` 将其生成 `status=completed` 的 Responses；同样参数无法通过入站重放，流式最终参数验证也会拒绝，形成协议不一致。
- 修复：在共用 `inboundResponseItem` 中验证 JSON object，保留空参数缺省 `{}`；不另建转换器或吞掉异常。
- 失败证据：`go test -p 2 ./internal/transformer -run '^TestResponsesOutboundRejectsNonObjectToolArguments$' -count=1` 退出 1，四个子用例均显示 `accepted: <nil>`。
- 通过证据：修复后 `go test -race -p 2 ./internal/transformer -run TestResponses -count=1` 退出 0（1.412s）；使用临时 HOME/XDG、只读模块缓存、禁下载，无真实上游。

## 审查边界

- 原协议子代理未回传可用结果，已停止；以上是主线程审查，不宣称额外的独立协议审查已完成。
- 实际 CLI 与全部相关 package 的最终验证另见父任务 PROGRESS；此处不把定向测试当成真实账户、所有高级能力或所有平台的验证。
