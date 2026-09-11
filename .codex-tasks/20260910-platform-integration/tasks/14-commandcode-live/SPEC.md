# CommandCode指定模型真实测试

single-full。用户明确指定deepseek/deepseek-v4.1-flash，验证通过本项目的Codex Responses及Claude Messages请求；本机Codex 0.144.3-cometix、Claude Code 2.1.263已确认可用。

使用隔离HOME/CLI配置与最小提示，复用远端已有认证，不导出真实Key，不替换用户全局模型。不购买套餐、不压测、不悄悄换模型。先验证两条最小请求，必要时仅为明确故障假设增加有限测试；记录是否实际CLI和是否流式，不能用模拟上游当真实成功。

验收：两条链路的HTTP/终止状态、请求model、平台归属、响应标记及Token日志有证据；无法调用指定模型时明确保留实际错误。只保存任务生成内容和白名单统计。
