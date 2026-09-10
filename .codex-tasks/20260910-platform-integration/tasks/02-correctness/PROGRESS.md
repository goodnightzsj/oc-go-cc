# Progress

## Context Recovery Block

- 当前：4/4 DONE；原始合成复现见 /tmp/oc-go-cc-routing-review.yHSDWm。
- 已验收：config单一凭证来源、provider+model身份、显式wire_format双向一致、保守账单同步和独立上游修复。
- 验证环境继承01-baseline。启动真实服务和真实账单不在范围。
- 下一步：父任务03协议终审、04面板与05文档验收。

## 恢复与补充验证

- 2026-09-10：主代理恢复既有修改并核实当前写入归属；CommandCode 指定参考仓库任务记录已存在，保留原进度。
- 隔离环境 `go test -race ./internal/config ./internal/router ./internal/client ./internal/provider`：config/router/provider 通过；client 的 TestOpenRouterEndpoint 仍断言旧的 `/api/v1` 路径，需结合真实 Chat Completions 请求合同修正验收，不隐藏失败。
- config.AtomicConfig 的 Reload/ApplyLoaded 共用写锁，Reload 在锁内读盘，防止旧监听结果覆盖新保存；新增并发回归通过 config race。
- GUI 局部配置对象替换会丢失未提交的 URL/key，已交 data_review 修复并验证；路由 maps 仍保留整段替换的删除语义。
- catalog 缺价改为保留 NULL 后，selector 发现未知价格被当作免费候选，已补 nil rate 过滤及明确免费回归（待下一轮测试）。

## 验收

- 2026-09-10：隔离HOME的全量race首次通过，覆盖本子任务全部包；`go vet -p 2 ./...`通过。证据：`/tmp/oc-go-cc-final-verify.3pcWpR/race.jsonl`、`vet.log`。
- 上述OpenRouter旧断言和未知报价选择问题均已修复并通过验证。账单合并保留不明归属及未匹配本地记录，不删除其他平台数据。
