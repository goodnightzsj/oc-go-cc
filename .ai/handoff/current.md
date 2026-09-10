# Current Handoff

## Task
- Name: 五平台完整适配、部署验收与后续 UI/UX 分析
- Goal: 继续全部平台任务；完成后提交推送和既有 SSH 部署，再分析页面美化。
- Owner CLI: Codex
- Support CLI: Claude 的本项目历史 session 仅作为部署流程证据。

## Status
- Current phase: Epic 5/7；详细状态以 CSV 为准。
- Current step: 06-deploy 1/3，SSH只读预检完成，准备提交、备份与部署。
- Done: 上游语义融合、路由/记账修复、CommandCode 双客户端、五平台独立查询与 OpenRouter 官方额度接口。
- Next: 提交推送 → 远端本地备份 → SSH部署测试 → UI/UX只读分析。
- Blockers: Zen/CommandCode 无公开账户查询合同；Bedrock 账单需要独立 IAM 权限；真实新平台账户未验证，不冒充已接入账户余额。

## Sources of Truth
- llmdoc: `llmdoc/startup.md`、`llmdoc/must/accounting-baseline.md`；此次扩展的llmdoc同步待用户确认。
- task files: `.codex-tasks/20260910-platform-integration/SUBTASKS.csv`、`tasks/06-deploy/TODO.csv`。
- key paths: `internal/gui/`、`internal/quota/`、`internal/storage/`、`docs/commandcode.md`、`tasks/06-deploy/deployment-flow.md`、`scripts/prod-deploy.sh`。

## Why Handoff
- Reason: 保留跨会话执行上下文；旧20260808任务已完成，不再作为当前任务真源。
- What the next CLI should do: 先读活动CSV及恢复块，保留失败证据和已完成工作。
- What it must not redo: 不读取凭证、不读取无关session、不覆盖未提交实现、不把旧验收结果作为当前源码验收、不在部署前开展视觉重设计。
