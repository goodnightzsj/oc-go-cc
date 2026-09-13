# Current Handoff

## Task
- Name: OpenCode历史恢复、CommandCode指定模型实测与平台排序
- Goal: 关闭远端清理、安全恢复可核实历史、测试deepseek/deepseek-v4.1-flash双客户端、统一OpenCode Go和CommandCode优先展示。
- Owner CLI: Codex
- Support CLI: Claude本项目最终session仅作为部署和官方同步/Token拆分证据；没有把真实凭证导出到本机。

## Status
- Current phase: Epic 15/16；正式d7b25fb已部署，自动清理关闭；#13官方6561条全量恢复与#15原Edge验收完成；#16结构化输出缺口已修复（源码+全量race/vet/build通过，尚未提交、尚未部署）。新增任务脚本、证据和文档尚未提交。
- Current step: 整理提交本轮恢复/验收证据、文档与#16修复；#8其余账户权限缺口仍保留。#14测试交付完成不等于上游逐模型已兼容。
- Done: 当前官方132非空页/明确空页/6561唯一ID已重新取得；事务补齐requests/provider_usage各6561，四类Token及$25.14740738与官方/原Edge分页/两个汇总API/SQLite一致。2337条保留原生详情，4224条未知详情，3条无唯一对应备份日志未额外计费。新启动PID893仍cleanup disabled，配置-1。
- Done: Go→CommandCode→Zen→Bedrock→OpenRouter在六个原生/渲染菜单及设置中通过；七页565检查/29组合/70布局/21截图/377只读请求全通过，0异常或意外写请求。原d7b25fb全量666/1103及六目标记录保留；本轮没改应用源码或再次部署。
- Done: Codex0.144.3-cometix返回CC_CODEX_OK（纯输入2723/缓存4352/输出6）；Claude2.1.263返回CC_CLAUDE_OK（输入144/输出21），均为指定模型和CommandCode归属。使用同release隔离实例；正式路由/账本未混入测试。临时凭证副本、进程、隧道已清理。
- Next: 提交本轮恢复/验收证据、文档与#16修复；是否部署需另行确认。原Edge driver仍在`/tmp/oc-go-cc-resume-20260912.SHViXM/edge-driver.mjs`，复用同一连接，结束后断开并提醒关闭调试许可。
- Blockers: #8为Go账户error、Zen无公开合同、Bedrock账单disabled、OpenRouter未配置。结构化输出本身已不阻塞：`output_config.format` 现映射为 `response_format`（证据 `tasks/14-commandcode-live/raw/structured-output-fix.json`），但上游是否逐模型接受 `response_format` 无官方承诺，且未做真实复测。不得忽略format或把未知费用伪装成功。

## Sources of Truth
- llmdoc: `llmdoc/startup.md`、`llmdoc/must/accounting-baseline.md`与最终计费审计；当前恢复方法详见`docs/history-recovery.md`。
- task files: `.codex-tasks/20260910-platform-integration/SUBTASKS.csv`及`tasks/13-history-restore/`、`tasks/14-commandcode-live/`、`tasks/15-provider-order/`。
- key paths: `docs/history-recovery.md`、`docs/troubleshooting-cost-mismatch.md`、`llmdoc/must/accounting-baseline.md`、`llmdoc/architecture/usage-pipeline.md`、`docs/commandcode.md`、`internal/transformer/request.go`、`pkg/types/openai.go`、`internal/config/provider_display.go`、`internal/storage/retention.go`。
- Remote evidence: `/root/oc-go-cc/.tmp/official-history-20260912.c9MOtJ/`含最终备份/计划/演练/事务报告；旧native-history-audit目录保留。生产验收摘要分别在task13和task15的raw/production-acceptance.json，完整核验在`/tmp/oc-go-cc-final-20260913.4mrEaW/`。不提交原始业务数据或凭证。

## Why Handoff
- Reason: 修正落后于子任务的交接状态，保留真实完成结果与未解除的权限/协议限制；不得从旧恢复块重写生产。
- What the next CLI should do: 先读活动CSV及恢复块，保留失败证据和已完成工作。
- What it must not redo: 不重复恢复6561条、不读取凭证或无关session、不覆盖用户改动、不清表/全库覆盖、不把旧TSV当当前全量、不重复主请求消耗、不回滚到会把-1当7天清理的旧程序、不用新浏览器冒充原Edge验收。
