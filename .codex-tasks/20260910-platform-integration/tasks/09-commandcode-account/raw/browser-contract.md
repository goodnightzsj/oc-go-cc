# CommandCode 套餐协议实证（2026-09-11）

## 授权与采集方式

用户明确授权复用 Edge 登录态查找套餐接口，并确认已在远端添加 API Key。使用 edge-debug-attach，单个 WebSocket 连接，未重启/新开浏览器；仅附加 CommandCode 页面。只保存本任务的协议与额度字段，不保存 Cookie、Key、账户 ID、付款方式或发票。

## 网页请求

- 页面路由：`/<account>/settings/usage` 与 `/<account>/settings/billing`。
- `GET https://api.commandcode.ai/internal/usage` → 200；列表 `usages: []`、`nextCursor: null`、`limit: 10`、`periodBasis: plan-window`、`window: {days: 1, entries: 100}`。这是有范围的列表，不能冒充完整历史。
- `GET /internal/usage/summary` → 200；`periodBasis: billing-period` 与数字汇总。
- `GET /internal/billing/credits` → 200；余额点数、`monthlyCreditsGranted` 与 `windowLimits`。
- `GET /internal/billing/subscriptions?withPending=true` → 200；有效套餐、周期、取消状态。
- 网页公开模块 `https://commandcode.ai/assets/use-billing-data-CIgHd5-E.js` 对这些请求使用 `credentials: include`。去掉登录态后，上述三个账户接口全部 401，提示需要登录。未读取 Cookie。

## API Key 请求来源与真实验证

官方页面模块 `https://commandcode.ai/assets/constants-CLU1yPTE.js` 同时定义：

| 常量 | 只读端点 |
| --- | --- |
| ALPHA.BILLING.CREDITS | `/alpha/billing/credits` |
| ALPHA.BILLING.SUBSCRIPTIONS | `/alpha/billing/subscriptions` |
| ALPHA.USAGE.SUMMARY | `/alpha/usage/summary` |

三项无认证请求均返回 401，提示无效 Authorization。官网 [GOAT 文档](https://commandcode.ai/docs/plans/goat) 说明 CLI 与 Provider API 使用同一 API Key。

远端临时探针使用本项目 `config.Load` 和既有服务 EnvironmentFile，由远端进程构造 `Authorization: Bearer`；Key 从未离开远端。第一次未显式指定配置路径，加载失败，未发起账户请求；按已确认服务路径显式设置 `ROUTATIC_PROXY_CONFIG` 后成功。真实配置、服务、数据库均未修改或重启。

- `/alpha/billing/credits` → 200：`credits {freeCredits: 0, monthlyCredits: 70, purchasedCredits: 0, belowThreshold: false, creditThreshold: 0}`；`windowLimits {limited: true, exceeded: null, fiveHour: {used: 0, cap: 14, exceeded: false, resetAt: 0}, weekly: {used: 0, cap: 35, exceeded: false, resetAt: 0}}`。
- `/alpha/billing/subscriptions` → 200：`success: true`；`data.planId: individual-goat`、`status: active`、`currentPeriodStart: 2026-09-10T07:35:57.000Z`、`currentPeriodEnd: 2026-10-10T07:35:57.000Z`、`cancelAtPeriodEnd: false`。身份和支付标识未保留。
- `/alpha/usage/summary` → 200：`totalCount/totalCost/averageCost/successRate/completedCount/failedCount/totalTokensIn/totalTokensOut/totalTokens/totalCredits/totalFreeCredits/totalMonthlyCredits/totalPurchasedCredits` 均为数字 0；`periodBasis: billing-period`。

## 单位与显示不变量

- [GOAT 官方文档](https://commandcode.ai/docs/plans/goat) 将额度描述为美元计价用量点数；它不是现金余额，也不是本项目本地估算费用。
- Alpha credits 响应**没有**网页 internal 响应的 `monthlyCreditsGranted`。不得硬编码 70 为所有套餐月限额，也不由当前剩余值推断月度百分比。显示月度剩余和官方周期即可。
- `windowLimits.fiveHour/weekly` 自带 `used/cap`，可独立计算使用比例；以 `exceeded` 显示是否达到窗口限制。
- 公开页面 `https://commandcode.ai/assets/usage-_d-IXX_9.js` 使用 `resetAt - Date.now()` 计算重置，因此 `resetAt` 是 Unix 毫秒；0 表示窗口尚无重置时间，不能显示 1970。
- 套餐/额度/汇总分块保留错误；一个失败不能清空其余已成功数据，多 Key 不合计余额。
- Alpha 为官方现有 CLI 接口，尚未作为稳定公开账户合同记录；界面标注 Alpha 数据源及可能变化，不再使用“没有账户接口”的绝对说法。
- 官方文档当前 Billing 与 Key 链接分别为 `/billing`、`/settings/keys`；无需硬编码用户路由。

## 浏览器收尾

原账户标签页在采集后不再存在，未重建或重登录；同一连接附加已存在的 GOAT 文档页补充公开重置单位证据。任务相关查询完成后已断开 CDP；应提醒用户取消 remote-debugging 允许开关。
