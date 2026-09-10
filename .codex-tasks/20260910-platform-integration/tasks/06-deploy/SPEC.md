# 提交、推送和 SSH 部署测试

## Task Shape

single-full；依赖 #5 最新实现验收。用户 2026-09-11 明确授权完成后部署。

## Goals

- 读取 SSH 运行手册，确认远端仓库、服务、现有版本与回滚路径，不读取真实凭证或配置内容。
- 审查并提交本任务改动，push origin main；按历史流程在 /root/oc-go-cc 拉取并运行 scripts/prod-deploy.sh。
- 核实远端提交、服务健康、页面/API 真实行为；失败时保留证据并使用既有回滚路径。

## Constraints

不覆盖无关远端修改，不删除生产数据，不显示凭证或完整私有请求；不在实现验收未完成时部署。

## Done-When

目标提交已运行，健康和页面验收通过，配置/数据保留；真实账户能力未验证范围明确。

## Validation

SSH 查询提交与 systemctl 状态，curl /health，浏览器/API 冒烟；仅服务启动不算完成。
