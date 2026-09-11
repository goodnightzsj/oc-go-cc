# 真实账户权限与额度验收收尾

## Task Shape

single-full；从原五平台真实账户验收要求显式拆出，避免将软件发布通过冒充账户数据全部可用。

## Goals

- 用户在自己的服务配置中提供具备权限的各平台凭证后，由应用执行只读额度查询并核对归属、时间、币种和错误状态。
- Go现有Key的用量接口403需要用户核对订阅/权限；OpenRouter需要平台推理Key，账户Credits另需Management Key；Bedrock官方SDK接口已实现，需要用户配置独立IAM身份/账户并授权收费查询。
- Zen/CommandCode缺少公开账户查询合同：保留缺口，等待官方合同或用户明确授权的替代数据来源，或由用户明确接受能力边界。

## Non-Goals / Constraints

不读取或索要用户明文凭证，不猜测私有接口，不绕过套餐权限，不用本地账本充当官方余额。真实付费推理需另获授权，不作为本轮只读部署验收的隐含步骤。

## Environment / Evidence

项目 `/Users/zsj/code/program/oc-go-cc`，运行代码 `0b28ab2`（2026-09-11部署验收）；软件和合成合同已验证，见 `../06-deploy/PROGRESS.md` 与 `../04-dashboard/raw/platform-quota-capabilities.md`。

## Done-When

可查询平台的授权账户实测通过；未公开或缺权限的能力有真实解决方式或用户明确确认的边界。未满足前不得标记全部账户接入完成。

## Validation

应用只读 `/api/quota?provider=...` 的实际账户结果与用户平台控制台核对；无授权/合同前不能自动验收。
