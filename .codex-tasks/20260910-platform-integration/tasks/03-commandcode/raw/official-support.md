# CommandCode 官方支持核实（2026-09-10）

## 决策

- Claude Code 调用 Claude 模型：使用官方 Anthropic Messages 原生接口，不另加反向工程代理。
- Codex：当前官方端点清单没有 Responses；Codex 当前自定义模型 provider 只支持 `wire_api="responses"`。目前没有官方依据确认可直连（不是经真实账户实测判定不支持），本项目按已公开合同增加 Responses 入站适配。
- Claude Code 调用非 Claude 模型：官方 `/messages` 不接受，需要本项目已有 Messages→Chat Completions 转换；不能把 native Claude 支持扩张到所有模型。
- 为保留本地日志与统计，推荐客户端→本项目→官方API；另提供客户端完全直连官方的示例，但该流量不会进入本地统计。

## 官方证据

1. [CommandCode Provider API](https://commandcode.ai/docs/provider)：列出的端点仅 `POST /provider/v1/chat/completions`、`POST /provider/v1/messages`、`GET /provider/v1/models`，根地址 `https://api.commandcode.ai`。Claude模型使用 Messages，OpenAI与其他模型用 Chat Completions，错用返回400。
2. 同页：各端点支持 Bearer；Messages另支持 `x-api-key`。流式支持 `stream:true`，末尾报告usage。`x-cmd-zdr:1`开启ZDR；无满足要求的上游时返回422，不能悄悄取消该约束。
3. [Codex官方手册](https://developers.openai.com/codex/codex-manual.md)，2026-09-10 helper确认当前缓存新鲜：provider的 `wire_api` 仅 `responses`；`model_providers` 的base_url/env_key等放用户级配置，不覆盖现有真实配置。定位：当前手册12260起与14612；[配置参考](https://learn.chatgpt.com/docs/config-file/config-reference)。
4. [Claude Code网关接入](https://code.claude.com/docs/en/llm-gateway-connect)：`ANTHROPIC_BASE_URL`指定网关，`ANTHROPIC_AUTH_TOKEN`发送Bearer；`ANTHROPIC_API_KEY`发送x-api-key。官方不保证通过网关调用非Claude模型的产品功能。
5. 公开模型端点无凭证GET实测成功：JSON顶层`object,data`，当时69个条目；模型包含`id/object/created/owned_by/name/context_length`，样本`claude-sonnet-4-6`。未见价格、账户套餐或余额字段，不能从模型清单推算账户额度。

## 套餐、账单与未知边界

- [使用额度文档](https://commandcode.ai/docs/resources/usage-limits)公开5小时/每周窗口与按需credit规则，查看途径为CLI `/usage`及[Studio Usage](https://commandcode.ai/usage)。已查页面未公布可调用的账户套餐/余额HTTP合同。
- Provider API文档写Go无API权限、GOAT/Pro/Max/Team可使用；[营销页](https://commandcode.ai/provider)仍写Provider专属，两者不一致。实现不硬编码套餐授权结论，以实际上游401/403和用户账户为准；不绕过Go套餐权限。
- 不从请求数、本地估算费用、Go平台额度或公开月费伪造CommandCode剩余额度。面板区分本地统计、未知报价与官方账单，提供官方Usage/Billing入口。
- 未使用真实凭证，没有验证实际账户权限、模型生成、账单扣款和客户端真实端到端效果；合成协议验收与真实平台验收必须分开陈述。

## 指定参考仓库

- [MAXeaglet/commandcode-proxy](https://github.com/MAXeaglet/commandcode-proxy)公开页：默认master，HEAD `487f219f9586b2a4ba7f7435eed7ee19dabc53ec`（2026-09-08），标注MIT。
- README仅列Chat Completions、Messages、Models、health，没有Responses入口。因此它也不能直接解决Codex协议缺口。
- 其CLI指纹/生命周期与私有alpha请求不是本轮官方接入路径；不复制客户端伪装、权限规避或安装脚本。可参考工具结果排序、SSE早输出/保活、缓存usage等公开协议行为，但不全量移植。

## 本轮复核

- 2026-09-10 再次读取上述 Provider API、Usage Limits 与 Claude Code 网关文档（HTTP 200）；官方仍只列出 Chat Completions / Messages / Models。
- Codex manual helper 返回 `local manual was already current`，配置示例明确 `wire_api = "responses"` 为唯一支持值；这是客户端协议要求，不表示任意 Chat Completions 服务可用。
- 范围为 CLI 模型调用。Claude Code 的网页/Slack 等托管入口不适用自定义网关配置，不能承诺“所有客户端平台/功能兼容”。
- 当前 [Codex 配置参考](https://developers.openai.com/codex/config-reference) 再次确认 `model_providers.<id>.wire_api` 唯一值是 `responses`。手册模型概览仍有 Chat Completions 弃用说明，与当前配置表存在历史文字差异；实现以配置合同为准，不推荐 `wire_api="chat"`。本机仅查询版本：`codex-cli 0.144.3-cometix`，没有加载真实账户进行验证。
