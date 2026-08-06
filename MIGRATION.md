# 迁移规划：oc-go-cc → routatic-proxy（整体跟随上游新架构）

> 状态：✅ 用户已确认"整体跟随上游新架构" + "接受改名为 routatic-proxy"。
> 本文档是 merge 前的一次性完整规划，供执行参考。本仓库将整体迁往
> `upstream/main`（`samueltuyizere/oc-go-cc`）的新架构。

## 1. 目标与边界

- **目标**：把本地 fork 演进为与上游 `upstream/main` 同构的新架构（Provider 抽象 + storage/SQLite + catalog + GUI Web 控制台 + OpenRouter + 自更新），并获得完整 UI。
- **项目名**：接受改名 **oc-go-cc → routatic-proxy**（模块路径、命令名、配置目录、GUI 标题均随之变更；上游仍向后兼容读旧 `oc-go-cc` 配置）。
- **对象**：本地独有特性中被上游淘汰的部分一并收敛，不再维护本地旧架构。

## 2. 迁移策略：git merge，而非手撒

本地历史：`2e8a5dc`（分叉点）+ 26 个本地 commit（含本轮移植 `56e0b43`）。
上游历史：`2e8a5dc` + 48 个 commit（HEAD `f4616cc`）。

采用 `git merge upstream/main`，保留完整历史（不 squash），在**新分支**上解决冲突后合入 main。

```bash
git checkout -b merge-upstream
git merge upstream/main          # 预演：25 个文件冲突（见 §3）
# 逐文件解决，倾向上游版本，仅补回本地独有价值（见 §4）
# go build ./... && go vet ./... && go test -race ./...
git commit -m "merge: adopt upstream architecture (routatic-proxy)"
# 校验通过后合入 main
```

## 3. 冲突面（已试运行确认）

- 上游改动 209 个文件，git 自动合并 184 个，**手工解决 25 个**。
- 冲突集中在双方都改过的核心文件：

| 冲突文件 | 说明 | 解决方向 |
|---------|------|---------|
| `internal/transformer/stream.go` / `stream_test.go` | 上游有更完善的 ProxyStream/Responses/Gemini | 用上游版本 |
| `internal/transformer/request.go` | 上游有 cache_control 处理 | 用上游，若缺本地 DeepSeek 增量则补回 |
| `internal/router/scenarios.go` / `model_router.go` / `fallback.go` | 上游有策略引擎/容量/selector | 用上游 |
| `internal/server/server.go` | 上游有 /models、/v1/models、statusline | 用上游 |
| `internal/client/opencode.go` | 上游有 provider 抽象 | 用上游 |
| `internal/handlers/messages.go` | 上游有 provider dispatch + 日志 | 用上游 |
| `internal/config/*` / `internal/config/model_registry.go` | 上游有 ResolveModelConfig + registry | 用上游（含我移植的 registry 之上游版本）|
| `internal/router/capacity.go` | 上游有 FilterByCapacity/clampOutputTokens | 用上游 |
| `internal/transformer/idle.go` | 上游有 StartIdleWatchdog | 用上游 |
| `Makefile` / `CLAUDE.md` / `CONFIGURATION.md` / `Dockerfile` / `cmd/main.go` / `.gitignore` | 文档/构建与重命名 | 用上游 |

> 我本机已把 idle.go / capacity.go / model_registry.go 移植到了本地；上游有同文件更成熟版本，
> merge 时统一用上游版本，无需保留本地移植（底层逻辑一致）。

## 4. 本地 26 个 commit 独有价值的处置

经对上核，本地绝大多数独有特性上游已有**等价或更成熟实现**：

| 本地独有 commit | 上游等价 | 处置 |
|----------------|---------|------|
| `/models` + `/v1/models` + 根 `/` 服务信息 | 上游 `a7a34cc` 已加 | 用上游 |
| DeepSeek cache_control / reasoning 精细处理 | 上游 request.go 有 | 用上游，merge 后对照补齐 |
| MiniMax 直连模型名注入 | 上游 provider 抽象 | 用上游 |
| model_overrides / respect_requested_model | 上游 model_overrides + family overrides | 用上游 |
| 请求日志 / source watcher | 上游 analytics / scripts | 用上游；`ops/systemd` 若保留 |
| 我移植的 capacity/SSE/auth 短路 | 上游天然有 | 用上游 |

**真正独有、需决定去留**：
- `ops/systemd/oc-go-cc*.service` —— 本地部署资产（systemd），上游无。**建议随 Rename 保留/改名为 routatic-proxy 服务**，或在迁移后单独评估。
- `scripts/dev-* / prod-*` ── 本地上游式的部署 watcher 脚本，上游用 `build_dmg/e2e-test` 等。按需取舍。

## 5. 验证步骤（merge 后必做）

1. `go build ./...`（新模块路径 `github.com/routatic/proxy`）
2. `go vet ./...`
3. `go test -race ./...`（含 storage/catalog/gui/updater 新包测试）
4. `go run ./cmd/routatic-proxy serve` 冒烟
5. 启动 GUI：`go run ./cmd/routatic-proxy gui`（macOS 原生窗口 + Web 控制台），确认 UI 出现
6. `git push` 到 origin

## 6. 产出（迁移后你将获得）

- **UI 页面**（用户关心的）：`internal/gui/` Web 控制台 —— analytics token 用量/延迟、性能面板、catalog 模型目录、配置编辑/导入导出、Quick Model Test；macOS 原生 systray/webview 入口。
- **新架构**：Provider 抽象（OpenCode Go/Zen/AWS Bedrock）、SQLite storage（analytics/catalog/latency/logs/requests/retention）、OpenRouter 支持、自更新 + 双通道、catalog。
- **项目名**：routatic-proxy（含全部改名）。

## 7. 风险

- 25 个冲突文件需逐一手工解决，若倾向上游则基本是"丢弃本地增量"，工作量大但机械。
- 合并后本地旧架构的既有测试（messages_test/stream_test 等）会与上游测试冲突，需以新架构为准重跑。
- 长期跟随上游需持续 merge 上游更新（建议固定 upstream remote）。
```
