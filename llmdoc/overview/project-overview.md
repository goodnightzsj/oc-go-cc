# 项目概览

routatic-proxy（仓库名 oc-go-cc）：位于 Claude Code / OpenCode 客户端与 OpenCode Go 网关之间的反向代理服务。接收 Anthropic Messages API 请求，路由到不同上游模型（OpenCode Go / OpenCode Zen / AWS Bedrock / OpenRouter），在 Anthropic ↔ OpenAI 格式间转换，并回传 Anthropic 风格的 SSE 流。附带 SQLite 持久化（请求历史、用量分析）与内嵌 Web 面板（`start` 模式监听 3445，代理监听 3456）。

## 核心特征

- **配置驱动路由**：模型与场景全部来自 `~/.config/routatic-proxy/config.json`；`internal/router/scenarios.go` 按请求特征（长上下文/复杂/思考/后台/默认）选择模型。
- **缓存 token 语义**：2026-08-26 修复后按 OpenAI 标准 `cached_tokens` 拆分缓存读取（详见 `architecture/usage-pipeline.md` 与 `reference/cache-billing-audit.md`）。
- **双发布通道**：main 分支自动发 beta（vX.Y-beta.N）；releases 分支手动发稳定版。
- **远端部署**：23.80.89.173，`git pull && bash scripts/prod-deploy.sh`（release 快照 + systemd）。

## 主要目录

| 路径 | 职责 |
|------|------|
| `cmd/routatic-proxy/` | cobra CLI（start/serve/costs 等） |
| `internal/config/` | 配置类型与 JSON 加载（env 插值） |
| `internal/router/` | 场景路由、模型链、断路器、成本路由 |
| `internal/transformer/` | Anthropic ↔ OpenAI 转换、SSE 转换、usage 拆分 |
| `internal/provider/` | 各上游 provider（流式/非流式发送） |
| `internal/storage/` | SQLite：requests、provider_usage、analytics、seed 价格 |
| `internal/gui/` | 内嵌仪表盘（HTML/CSS/JS） |
| `internal/debug/` | debug_capture（原始请求/响应记录） |
| `pkg/types/` | 共享 wire 类型（Anthropic/OpenAI/Normalized） |

## 直接相关文档

- `startup.md` — 上手路径
- `architecture/usage-pipeline.md` — 用量与缓存 token 链路
- `reference/cache-billing-audit.md` — 缓存/计费修复与三端对账记录