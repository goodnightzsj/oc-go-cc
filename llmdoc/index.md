# llmdoc 索引

routatic-proxy（oc-go-cc）：Claude Code ↔ OpenCode Go 反向代理 + 用量面板。启动顺序见 `startup.md`，本页不重复。

## 稳定文档

| 文档 | 内容 |
|------|------|
| overview/project-overview.md | 项目定位、核心特征、目录地图 |
| architecture/usage-pipeline.md | 用量/缓存 token 链路（入口→路由→发送→转换→录制→成本→下游） |
| reference/cache-billing-audit.md | 2026-08-26 缓存与计费修复、三端对账（OpenCode/代理/CompactGate） |
| must/accounting-baseline.md | 记账/调试硬约束（缓存语义、seed 价格同步、capture 约定、部署与时区底线） |

## 目录导航（未建文档的领域转查 CLAUDE.md 与源码）

- `cmd/routatic-proxy/` — cobra 入口：start/serve、costs（对账工具链）、catalog、models、update/update-channel、autostart
- `internal/config/` — Config 21 个顶层字段；example.json 缺 `api_keys`/`openrouter`/`catalog`/`storage` 示例
- `internal/router/` — 场景路由、模型链、断路器、成本路由（场景列表 `config.CostScenarioNames`）
- `internal/storage/` — SQLite 5 表：requests / provider_usage / schema_info / providers / models
- `internal/gui/` — 内嵌面板（index.html + app.js + style.css，Tailwind 编译）
- `internal/debug/` — debug_capture（详见 must/accounting-baseline.md）
- `internal/provider/` + `internal/client/` — 上游适配与捕获（见 architecture/usage-pipeline.md）
- `pkg/types/` — Anthropic/OpenAI 共享 wire 类型
- 交付面：`.github/workflows/`（ci / beta-release / release / release-pipeline）+ `scripts/`（prod-deploy 等）+ 双通道发布（见 CLAUDE.md）

## Memory

- reflections/2026-08-26-cache-audit.md — 本次对账反思（教训与晋升）
- decisions/ — （暂无）

## Gaps（首次 init 未覆盖）

- router 场景决策与成本路由的深度文档
- transformer 全部转换契约（thinking/effort、tools 格式）
- GUI 面板功能面与 API 端点清单
- 发布/部署流水线文档（已有 CLAUDE.md 覆盖大部分）