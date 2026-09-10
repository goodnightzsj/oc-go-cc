# 平台与账单正确性

- 形态：single-full；依赖：01-baseline。
- 目标：平台密钥单一来源；明确格式同时控制发送和解析；fallback/去重/记账以provider+model为身份；不根据不完整账单删除无归属证据的行。
- 约束：保留旧四平台global key回退；未知provider拒绝；不读取真实配置/DB；保留缓存token拆分；上游补丁仅按现接口移植。
- 验收：合成凭证、httptest、临时DB覆盖关键成功/失败行为；相关race测试通过。
- 验证：go test -race ./internal/storage ./internal/provider ./internal/router ./internal/config ./internal/client ./internal/handlers ./internal/transformer。
