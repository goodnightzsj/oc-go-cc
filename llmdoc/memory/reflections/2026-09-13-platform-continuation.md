# 反思：2026-09-13 平台接入续做与仓库瘦身

**任务**：续做 Epic `20260910-platform-integration` 的未闭合问题（结构化输出兼容、上游 delta 复核、未提交清理），并整理仓库跟踪面。

## 做对的事

- 没有采信交接摘要：先读本目录 Codex 会话日志的终止事件，才发现它停在 `turn_aborted`（spawn 的子代理 11.5 小时未返回后被用户中断），而摘要写的是"当前复核缺口"。
- 上游 delta 逐条落到本仓库代码判断，而不是按 commit 标题归类：`3abc2e1` 的三处修复，两处本仓库已用不同实现达成（并有测试覆盖），一处本仓库根本没有对应代码，因此结论是"无需移植"。
- 结构性改动先确认唯一权威入口再动手：`output_config.format` 的映射改动前先验证 `AnthropicToChatCompletion` 委托 `TransformRequest`，四个 provider 与 Messages 入口共用同一条路径。
- 仓库瘦身时按"移出跟踪 ≠ 删除文件"执行，用户两次明确要前者；磁盘文件全部保留。

## 教训

- **交接状态与运行日志会不一致，且两个方向都可能偏**。摘要可能落后（实测进度已更新到 15/16）也可能超前（把"已定位缺口"写成"正在复核修复"）。真实状态以会话日志的终止事件 + 子任务 CSV 为准。
- **"可移出"清单必须先查引用再下结论**。我最初把 `rules/auto-detected/` 和 `docs/platform-integration-review.md` 都列为可移出项，查引用后两项都改判：前者是上游文件且被 `REVIEW.md` 引用 16 次，后者被 4 个文件 5 处引用（含锚点）。
- **`git add -A <被 .gitignore 命中的路径>` 会整条命令失败**，不是部分成功；因为串在 `&&` 链里，后续 commit 被静默跳过，只留下"以为提交了"的假象。被忽略的路径不应出现在 `git add` 参数里。
- 库依赖漂移（`x/sys 0.46→0.48`、`sqlite 1.53→1.58`）在上游是多次独立 bump，本轮没有顺手升级：它不属于当前问题范围，会污染 diff 且需要重跑全量回归。

## 晋升候选

- `must/accounting-baseline.md`：保留策略负数语义与"不得回滚到旧程序"（数据完整性硬约束）。已晋升。
- `architecture/provider-layer.md`：五平台在配置/显示层，运行期只有四个 `core.Provider`；OpenRouter 走 legacy client。这个区分不看代码必然会读错。已建。
- `reference/inbound-protocols.md`：三个入站面与 Responses 适配器的 fail-closed 边界。已建。
- `architecture/model-routing.md`：override 优先级与成本场景键的唯一权威点。已建（原 `index.md` Gaps 已列此项）。
