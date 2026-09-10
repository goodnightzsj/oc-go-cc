# 上游核实与采纳计划（2026-09-10）

来源：https://github.com/samueltuyizere/oc-go-cc 。已 fetch，HEAD=b214eeb279d9a397872bbc0795c2486e3a0dd969；fork=684235d，origin一致。共27项未融合提交。此处记录筛选，不表示下列补丁已经全部实现。

最终状态：下列筛选已完成，采纳项按行为移植并通过最终回归；不是整提交cherry-pick。实际采纳/保留边界及验证见 `docs/platform-integration-review.md` 和父级 PROGRESS；远端HEAD再次核实未变化。

## 本轮语义移植

- b82c865：provider名称/override校验；不混入同提交的updater行为改造。
- e181e0a：Go Responses和显式wire_format；发送/解析统一，并补足Zen/Bedrock。
- ee74c4a：Responses工具调用及输入转换。
- 722ff60：Responses终止用量，保留fork缓存拆分。
- eab63d8：平台专属请求头。
- 744895c：Go会话ID转发；不将Go身份头发给CommandCode。
- 7cf5012：Zen模型协议分类前缀。
- bc6cda7：macOS Homebrew稳定自启路径。
- 478ec00：README移除已不存在ui命令。
- 0b723b2：仅审阅fork e3aebdc之外的thinking增量；已等效实现的不重复搬运。
- b64f155：仅P95/P99排序与nearest-rank修复；不搬有损异步记账队列、Normalized Blocks重构、token LRU、catalog后台刷新。

## 暂不混入

- 56d1f43：RPM/流水线/多模块迁移不属本轮，watcher就绪改动仅在确认相关失败时采用。
- Go依赖5项：23d4f5f、9761fea、91b24d4、07f5061、47f1259；本轮不需要新增API，SQLite升级须独立验证。
- CI依赖10项：d6c9066、14e61f2、d74b160、b214eeb、f088c76、0b3dd36、471bb86、3d3c72c、7c51580、3004d33；fork已经合并发布流水线，不能覆盖旧工作流。

## 必须保留

fork的原始Anthropic provider输入、配置所有的cost scenario、缓存两种格式、capture Close语义、中断流记账、发布流水线和可选CSS构建。尤其禁止用upstream整文件覆盖transformer/stream.go、response.go、normalized_bridge.go；upstream仍缺fork的缓存拆分修复。
