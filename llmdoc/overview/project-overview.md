# 项目概览

routatic-proxy（仓库名 oc-go-cc）：位于 Claude Code / Codex / OpenCode 客户端与上游模型网关之间的反向代理。接收 Anthropic Messages、OpenAI Responses 与 OpenAI Chat Completions 三种入站请求，路由到不同上游模型，在四类 wire format 间转换，并按入站协议回传对应形态的 SSE。上游在**配置与显示层**有五个平台（OpenCode Go、CommandCode、OpenCode Zen、AWS Bedrock、OpenRouter），运行期只有四个 provider 实现——OpenRouter 走 legacy client，详见 `architecture/provider-layer.md`。附带 SQLite 持久化（请求历史、用量分析）与内嵌 Web 面板（`start` 模式监听 3445，代理监听 3456）。

## 核心特征

- **配置驱动路由**：模型与场景全部来自 `~/.config/routatic-proxy/config.json`；场景判定见 `architecture/model-routing.md`。
- **缓存 token 语义**：2026-08-26 修复后按 OpenAI 标准 `cached_tokens` 拆分缓存读取（详见 `architecture/usage-pipeline.md` 与 `reference/cache-billing-audit.md`）。
- **入站三协议，一个管线**：Responses 入口是 Messages 管线的无状态适配器，边界 fail-closed，见 `reference/inbound-protocols.md`。
- **双发布通道**：main 分支自动发 beta（vX.Y-beta.N）；releases 分支手动发稳定版。
- **远端部署**：23.80.89.173，`git pull && bash scripts/prod-deploy.sh`（release 快照 + systemd）。

## 主要目录

| 路径 | 职责 |
|------|------|
| `cmd/routatic-proxy/` | cobra CLI（start/serve/costs 等） |
| `internal/config/` | 配置类型与 JSON 加载（env 插值） |
| `internal/router/` | 场景路由、模型链、断路器、成本路由 |
| `internal/transformer/` | Anthropic ↔ OpenAI 转换、SSE 转换、usage 拆分 |
| `internal/provider/` | 四个已登记 provider（Go / Zen / Bedrock / CommandCode），见 `architecture/provider-layer.md` |
| `internal/storage/` | SQLite：requests、provider_usage、analytics、seed 价格 |
| `internal/gui/` | 内嵌仪表盘（HTML/CSS/JS） |
| `internal/debug/` | debug_capture（原始请求/响应记录） |
| `pkg/types/` | 共享 wire 类型（Anthropic/OpenAI/Normalized） |

## 直接相关文档

- `startup.md` — 上手路径
- `architecture/usage-pipeline.md` — 用量与缓存 token 链路
- `architecture/provider-layer.md` — 五平台 vs 四个 provider、wire format、展示顺序
- `architecture/model-routing.md` — 场景判定、override 优先级、成本路由
- `reference/inbound-protocols.md` — 三个入站面与 Responses 边界
- `reference/cache-billing-audit.md` — 缓存/计费修复与三端对账记录