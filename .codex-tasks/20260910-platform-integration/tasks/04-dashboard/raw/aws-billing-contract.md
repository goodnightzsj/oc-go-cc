# AWS 账单增量合同（2026-09-11）

恢复源码核验：`internal/gui/quota.go` 的 AWS 分支只返回 `aws_billing_auth_required`，没有 Cost Explorer 客户端。该软件缺口现纳入 #4，不能全部推给用户权限。

## 已核实一手来源

- [Cost Explorer API](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-api.html)：独立 IAM 查询权限，服务端点 `https://ce.us-east-1.amazonaws.com`，官方建议使用 SDK 处理签名。
- [GetDimensionValues](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetDimensionValues.html)：支持 `SERVICE`、`COST_AND_USAGE`、SearchString、Filter、NextPageToken；不能用仅适用于 CostCategoryRule 的 `SERVICE_CODE` 代替。
- [GetCostAndUsage](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetCostAndUsage.html)：区间起日包含、终日排除；可用 SERVICE + LINKED_ACCOUNT 过滤；DAILY 和 UnblendedCost；返回金额字符串、单位、Estimated 与分页 token。组织管理账户可能有所有成员数据，因此本功能要求显式 linked_account_id。
- [Bedrock 成本归属](https://docs.aws.amazon.com/bedrock/latest/userguide/cost-management.html)：官方账单是按日/用量类型汇总，不能冒充每请求明细或本实例费用。
- [Cost Explorer Pricing](https://aws.amazon.com/aws-cost-management/aws-cost-explorer/pricing)：主账单视图每请求 USD 0.01；不支持本轮自定义 Billing View，分页另产生请求。页面须提示收费，不将轮询或平台切换转成收费操作。
- [Go SDK 配置](https://docs.aws.amazon.com/sdk-for-go/v2/developer-guide/configure-gosdk.html)：标准 AWS 凭证链及命名 profile；不复用本项目任何平台推理 Key。凭证来源的实际优先级以所安装 SDK 代码为准，不读取用户的 AWS 凭证。

## 实现和验证边界

默认关闭。用户显式提供账单账户范围，使用服务进程的 AWS SDK 身份；手动查询最近30个完整UTC日，先发现带 Bedrock 名称的服务再精确过滤；响应公开服务范围，无法由公开名称归属的其它服务/Marketplace项目不归入该合计。无匹配数据明确显示未找到，不伪造零余额。

结果为官方 UnblendedCost 而非可用余额；保留负费用、返回币种和 Estimated。分页失败或无效字段显式失败，不发布部分合计。缓存只按独立账单配置与UTC日期复用，自动访问不查询上游。所有回归使用合成凭证和 HTTP；不启用远端 IAM、Cost Explorer 或任何收费查询。

已查询模块版本：`github.com/aws/aws-sdk-go-v2/config v1.33.4`、`github.com/aws/aws-sdk-go-v2/service/costexplorer v1.72.0`（2026-09-09 发布，最低 Go 1.24；项目 Go 1.25）。
