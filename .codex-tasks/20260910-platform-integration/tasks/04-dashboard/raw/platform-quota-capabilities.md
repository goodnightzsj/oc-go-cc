# 五平台账户数据合同核对

核实：2026-09-11；仅官方公开文档和合成 HTTP，未读取真实凭证或请求真实账户。

| 平台 | 已核实数据合同 | 本项目行为与缺口 |
| --- | --- | --- |
| OpenCode Go | 既有 `/zen/go/v1/usage`（未公开稳定 API 合同）及 Go 文档的模型额度 | 保留每 Key 窗口；文档额度与本实例模型费用分开，未知费用不计算完整占比，多 Key 不冒认单账户使用量 |
| OpenRouter | `GET /api/v1/key` 使用推理 Key；`GET /api/v1/credits` 需要 Management Key | 实现两种查询，管理 Key 独立配置/脱敏，绝不用于推理；每 Key 限额不当作账户余额，不合计多 Key，BYOK 分列，缺字段为 null、负余额保留 |
| OpenCode Zen | 公开推理端点、按量付费、工作区/月限额和控制台，所核对文档未公开账户查询 API | 五平台统一能力响应 + 独立本地账本；账户余额仍未接入，不推测 Go usage 适用于 Zen，不请求私有控制台 |
| CommandCode | Provider API 公开 Messages、Chat Completions、Models；Usage Limits 描述 CLI/Studio 查看方式，未公开账户余额 REST 合同 | 五平台统一能力响应 + 独立本地账本；保留 Usage/Billing/Keys 入口，不把“入口存在”算作余额已接入 |
| AWS Bedrock | 推理 API Key 是 Bedrock Bearer 认证；Cost Explorer 是另一个账户级服务，需要显式 IAM 查询权限 | 明确 `aws_billing_auth_required`，提供独立本地账本及账单控制台；AWS 账户账单仍未接入，不把推理 Key 用于 Cost Explorer、不假造套餐余额 |

## 一手来源

- [OpenRouter credit/rate limits](https://openrouter.ai/docs/api_reference/limits)：Key cap、剩余额度、UTC 日/周/月用量、BYOK、free tier。
- [OpenRouter credits](https://openrouter.ai/docs/api/api-reference/credits/get-remaining-credits)：`total_credits`、`total_usage`，403 说明 Management Key 要求。
- [OpenCode Zen](https://opencode.ai/docs/zen/)：Endpoints / Pricing / Monthly limits / Roles。
- [CommandCode Provider API](https://commandcode.ai/docs/provider)：Endpoints / Streaming / Errors / API support。
- [CommandCode Usage Limits](https://commandcode.ai/docs/resources/usage-limits)：CLI `/usage`、5小时与每周窗口、pay-as-you-go 与信用余额；不是 HTTP 余额 API。
- [AWS Cost Explorer API](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-api.html)：账单服务端点和显式 IAM 授权要求。
- [AWS Bedrock API keys](https://docs.aws.amazon.com/bedrock/latest/userguide/api-keys.html)：Bedrock Bearer Key 与 AWS credentials 的区别。由两份 AWS 文档推断不能将现有 Bedrock Key 当作 Cost Explorer 授权。
- [OpenCode Go](https://opencode.ai/docs/go/)：模型额度文档，既有解析器来源；未把公开文档表误称为实时账户数据。

## 验收边界

- 接入完成表示已实现代码、独立配置、范围隔离和合成合同验证，不等于真实账户余额已验证。
- 三个平台的账户数据缺口必须在产品和交付报告中保留；不能把本地统计、官方链接或无公开合同伪装成实时账单。
- 所有平台的本地用量只统计经过本实例的流量；`GET /api/analytics/summary?provider=...&days=...` 与官方查询独立失败、独立显示来源。
- OpenRouter `/credits` 需要用户自行配置 `openrouter.management_api_key` 或 `ROUTATIC_PROXY_OPENROUTER_MANAGEMENT_API_KEY`，没有默认回退；账户可能与其它推理 Key 不同，所以不自动建立跨 Key 账户归属。
