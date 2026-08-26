# 反思：2026-08-26 缓存对账

**任务**：对齐远端代理计费与 OpenCode 账单，修复缓存计数丢失。

## 做对的事

- 用真实上游请求（最小 token 成本）验证字段结构，而非猜测格式；后续改用完整 debug_capture 抓真实流量确认。
- 把对账建立在用户提供的账单数据 + 远端 DB 三方交叉验证上，价格反推多行联立解决（误差 <0.1%）后才落 seed。
- 修复分三层递进：capture 不可用（两处 bug）→ 解析缺字段 → 价格过时，每层都有实证。

## 教训

- 最初误判"上游不透传缓存"：84-token 无缓存请求的 `prompt_tokens_details: {}` 让人误以为上游从来不返回——直到抓到大流量请求才看到 `cached_tokens`。**小样本探针不足以判断字段存在性**。
- debug_capture 自称"request/response capture"，但 registry 重构（39199fd/3c10460）后旧 client 路径已不可达，capture 成了死代码；代码审查必须验证"声称的路径是否真的被调用"。
- `teeReadCloser` 的管道泄漏是"Close 语义"类 bug 的典型：包装类覆盖 Read 就漏了 Close。补的回归测试（`TestCaptureBodyCompletesOnClose`）是防复发的最小保障。
- 时区坑：requests 存 +08:00、账单/CompactGate 存 UTC，两次查询因时区不匹配得到 0 行，必须显式换算。

## 晋升候选

- `llmdoc/must/accounting-baseline.md`（已建）：缓存语义、价格同步要求、时区与部署底线。