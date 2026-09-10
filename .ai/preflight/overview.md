# 多平台可靠性与 CommandCode 接入

- 范围：修复 2026-09-10 审查确认的问题；检查 origin/upstream 新提交并移植有价值的修复；为 CommandCode 增加独立平台配置、发送路径、路由、界面和统计身份；分析 Codex / Claude Code 接入。
- 架构：保留 Go HTTP 代理、Anthropic Messages 入口、SQLite 和内嵌 JS 面板，不更换技术栈。
- 优先级：数据完整性与凭证隔离 > 协议与路由正确性 > 数据展示 > 接入文档。
- 非目标：不部署、不推送、不操作真实账户或生产数据库，不修改全局客户端配置，不实现未经公开文档证实的 CommandCode 额度接口。
- 验收：已复现缺陷有回归测试；CommandCode 的流式/非流式、工具调用、密钥选择、模型路由、配置读写及统计通过本机模拟上游测试；全量 race/vet/六目标构建和界面冒烟有明确结果。

执行真源：`.codex-tasks/20260910-platform-integration/SUBTASKS.csv`。
