# AWS Bedrock 独立账单配置

套餐页已接入 Cost Explorer 的官方服务费用查询，并与本实例的请求、Token 和估算费用分开展示。此功能默认关闭，不会使用 Bedrock 推理 Key，也不会通过页面轮询产生收费请求。

## 配置与身份

在设置的 AWS Bedrock 区块启用手动账单查询，填写 12 位 AWS 账户 ID；可选填服务进程可访问的 AWS SDK profile。也可将以下片段合入现有配置：

```json
{
  "aws_bedrock": {
    "billing": {
      "enabled": false,
      "profile": "billing-readonly",
      "linked_account_id": "123456789012"
    }
  }
}
```

示例账户 ID 不是实际账户；替换后再启用。保留已有推理设置，不要直接用片段覆盖完整配置。

| 字段 | 含义 | 环境覆盖 |
| --- | --- | --- |
| `enabled` | 显式允许手动收费查询，默认 `false` | `ROUTATIC_PROXY_AWS_BILLING_ENABLED` |
| `profile` | 可选命名 profile，不填则使用服务进程的标准 SDK 凭证链 | `ROUTATIC_PROXY_AWS_BILLING_PROFILE` |
| `linked_account_id` | 查询必须限定的 12 位账户 ID | `ROUTATIC_PROXY_AWS_BILLING_LINKED_ACCOUNT_ID` |

身份由官方 Go SDK 解析，可使用服务进程已有的短期凭证、角色或命名 profile。本项目不新增 Access Key/Secret Key 表单，也不把任何平台推理凭证送入账单客户端。显式 `profile` 按 SDK 的命名配置优先级解析，未指定时才使用标准默认链。身份需要 `ce:GetDimensionValues`、`ce:GetCostAndUsage` 及对应账户/账单视图访问权限，并已启用 Cost Explorer；组织管理账户可能看到多个成员，所以应用始终附加 `LINKED_ACCOUNT` 过滤。该过滤不是 IAM 权限控制的替代品。[SDK 身份配置](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)、[Cost Explorer API 权限](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-api.html)。

## 如何查询

1. 保存独立账单设置；此时不查询 AWS。
2. 在套餐页选择 AWS Bedrock，点击“查询 AWS 账单（收费）”。仅该按钮发送显式 POST。
3. 查看费用、币种、账户范围、UTC 日期、纳入合计的服务名、按日费用和 AWS 预估标记。

主账单视图 API 按请求收费，当前官方价格为每请求 USD 0.01，分页也产生请求。有匹配服务时，服务名发现和费用查询通常至少需要两次请求；本实现两种操作各最多 20 页，不启用自定义 billing view，不自动重试收费请求。[AWS 官方价格](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing/)。

切换平台、定时刷新和普通“刷新”只读快照，不触发 AWS 查询。快照最多保存 24 小时，同 UTC 日、profile、账户范围才可复用；重启后没有快照，须再次手动查询。显式按钮可重新取数。禁用配置后立即停止提供旧账单；查询失败不会冒充成功或保留旧金额当作新结果。

## 金额与覆盖边界

- 查询最近 30 个完整 UTC 日，起日包含、终日排除。指标为 `UnblendedCost`，不是账户余额、最终发票或本实例估算。
- 通过 `GetDimensionValues` 发现名称含 Bedrock 的 `SERVICE`，再按精确服务名和账户 ID 查询；页面列出实际范围。不包含名称无法归属到 Bedrock 的其它服务或 Marketplace 项目，不声称覆盖账户所有费用。
- 保留 AWS 返回的币种、负费用、真实零值和 `Estimated`。不自动转换为本地 USD；预估值可能延迟或修订。
- 无匹配服务/费用时显示“未返回匹配数据”，不显示零余额。分页失败、重复日、缺失金额或混合币种会显式失败，不发布部分合计。

合同：[GetDimensionValues](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetDimensionValues.html)、[GetCostAndUsage](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetCostAndUsage.html)、[Bedrock 成本管理](https://docs.aws.amazon.com/bedrock/latest/userguide/cost-management.html)。接口响应见 [Dashboard API](reference-api.md#post-apiquotaprovideraws-bedrockbilling_refresh1)。

## 验证与真实账户验收

自动测试使用独立临时身份和合成 AWS HTTP 响应，覆盖 SDK SigV4、环境/profile 优先级、账户过滤、分页、币种、未知/零/负值、错误脱敏、GET/跨站请求禁止收费、局部配置保存和真实浏览器交互。部署默认不启用 AWS 查询、不创建 IAM 权限；真实账户需由账户持有人配置并授权后单独验收。
