# llmdoc 索引

routatic-proxy（oc-go-cc）：Claude Code / Codex ↔ 上游模型网关的反向代理 + 用量面板。启动顺序见 `startup.md`，本页不重复。

## 稳定文档

| 文档 | 内容 |
|------|------|
| overview/project-overview.md | 项目定位、核心特征、目录地图 |
| architecture/usage-pipeline.md | 用量/缓存 token 链路（入口→路由→发送→转换→录制→成本→下游） |
| architecture/provider-layer.md | 五平台 vs 四个已登记 provider、wire format 推断、平台展示顺序 |
| architecture/model-routing.md | 场景判定、override 优先级、成本路由与场景键的同步点 |
| `docs/site-architecture.md`（仓库根，非 llmdoc 内）| 站点可插拔架构设计（已定稿未实现）：描述符、current-site 切换、目录优先解析、`/v1/models` 缺陷与五阶段推进 |
| reference/inbound-protocols.md | 入站路由表、Responses 适配器与 fail-closed 边界 |
| reference/cache-billing-audit.md | 2026-08-26 缓存与计费修复、三端对账（OpenCode/代理/CompactGate） |
| must/accounting-baseline.md | 记账/调试硬约束（缓存语义、seed 价格同步、capture 约定、保留策略、部署与时区底线） |

## 目录导航（未建文档的领域转查 CLAUDE.md 与源码）

- `cmd/routatic-proxy/` — cobra 入口：start/serve、costs（对账工具链）、catalog、models、update/update-channel、autostart
- `internal/config/` — Config 21 个顶层字段；example.json 缺 `api_keys`/`openrouter`/`catalog`/`storage` 示例
- `internal/router/` — 场景路由、模型链、断路器、成本路由（场景列表 `config.CostScenarioNames`）
- `internal/storage/` — SQLite 5 表：requests / provider_usage / schema_info / providers / models
- `internal/gui/` — 内嵌面板（index.html + app.js + style.css，Tailwind 编译）
- `internal/debug/` — debug_capture（详见 must/accounting-baseline.md）
- `internal/provider/` — 四个已登记 provider；`internal/client/` 是 OpenRouter 与所有未登记 provider 的第二条路径（见 architecture/provider-layer.md）
- `pkg/types/` — Anthropic/OpenAI 共享 wire 类型
- 交付面：`.github/workflows/`（ci / beta-release / release / release-pipeline）+ `scripts/`（prod-deploy 等）+ 双通道发布（见 CLAUDE.md）

## Memory

- reflections/2026-08-26-cache-audit.md — 缓存对账反思（教训与晋升）
- reflections/2026-09-13-platform-continuation.md — 平台接入续做、上游 delta 复核与仓库瘦身反思
- decisions/ — （暂无）

## Gaps

- transformer 全部转换契约（thinking/effort、tools 格式）：目前只覆盖结构化输出与 Responses 入站
- GUI 面板功能面与 API 端点清单
- 发布/部署流水线文档（已有 CLAUDE.md 覆盖大部分）
- `internal/client/opencode.go` 作为第二条发送路径尚无独立文档页