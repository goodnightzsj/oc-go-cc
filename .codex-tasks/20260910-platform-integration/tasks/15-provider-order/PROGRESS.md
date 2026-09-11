# Progress

## Context Recovery Block

- single-full，2/3；CLI与模型API及GUI展示排序已实现，完整门禁通过，待发布及原Edge验收。
- 旧只读入口委托的解码失败记录保留；本次protocol_gate只读研究真实双客户端测试、backend_audit仅写GUI资产和对应排序测试，不写路由/存储/任务记录。
- 新增config.CompareProviderDisplay用于纯展示；CLI帮助/校验/模型列表和GET /v1/models排序一致。catalog错误提供方列表也统一并去除重复标签，不改变解析候选、fallback或密钥顺序。
- 回归首次因新测试误用嵌入Catalog类型编译失败；修正测试构造后明确复现旧alphabetical顺序和重复标签，再修复展示。配置/storage/catalog/handlers/CLI定向race全部通过；证据`/tmp/oc-go-cc-retention-verify.LW3OP1/targeted.log`。
- 下一步：与retention负值禁用同次部署。Edge握手受阻，线上视觉验收不得冒充完成。

## 2026-09-12 发布门禁

- 保留前序GUI实现，主线程没有重复改写app.js/index.html。旧代理未回传有效结论，已中断；新release_review仅只读审查。
- 首次全量只有`TestDashboardPlatformDataBehavior`两时区失败：旧测试假设平台随费用排名交换位置，与新的固定平台顺序冲突。改为按平台身份比较颜色，并检查顺序固定和颜色互异。
- 第一次测试修订的正则跨两层JavaScript模板转义后失配，定向仍失败；改为无转义歧义的`[^]*?`，定向两时区和平台排序均通过。两次失败保留，不归为产品颜色错误。
- 最终隔离HOME全量：666顶层/1103含子测试PASS、18包、0FAIL；`go vet`、六目标CGO=0构建、显式Codex合成工具双轮、JS语法和diff检查通过；源码哈希前后一致。
- 证据：`/tmp/oc-go-cc-recovery-20260912.S23g9t/`的race.jsonl、gui-order-fixed.log、gui-order-final.log、race-final.jsonl、vet-final.log、codex-synthetic.jsonl及sources.*.sha256。浏览器fixture未启动，原Edge尚未握手。
