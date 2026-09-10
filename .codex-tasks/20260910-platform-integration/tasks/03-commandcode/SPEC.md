# CommandCode 独立平台与客户端接入

## Task Shape

- single-full；官方支持核实后独立代码可先行，最终集成验收依赖 02-correctness。

## Goals

- 首先读取 CommandCode 官方文档，对 Codex Responses 与 Claude Code Messages 分别给出原生支持、认证、模型、流式、工具和 usage 证据。原生支持就直接接官方接口；无原生支持的链路才使用反代。
- 反代参考用户指定的 https://github.com/MAXeaglet/commandcode-proxy；如采纳须固定提交与许可证。不以第三方宣称替代官方原生支持判断。
- 完成 commandcode 独立平台配置、环境变量、CLI、provider 注册、模型路由；GUI/统计由后续 #4 验收。
- 分别核实并打通目标支持范围内的 Codex Responses 和 Claude Code Messages 链路；端点、模型、工具调用、usage 和错误行为均有测试或明确限制。
- 直接调用官方 API 不取消本项目日志、套餐查看、统计与独立平台设置；客户端绕过本项目直连的请求不能假称已被本地记录，套餐能力由正式文档决定。

## Non-Goals / Constraints

- 不安装或运行第三方安装脚本，不读取真实账户/凭证，不修改用户客户端全局配置，不杜撰额度或账单接口。
- 复用既有 Go HTTP、转换器和依赖；按许可证与实际合同移植行为，不整体替换本项目转换器。
- 已有四个平台配置和缓存拆分保持兼容；commandcode 凭证和头部不得混用其他平台。
- 参考代理与本项目 upstream 分别审阅，配置示例明确拓扑；只完成上游 Responses 转换不能宣称 Codex 可直连。

## Done-When

- 参考来源、已验证事实、采纳项与限制可追溯。
- 先完成 raw/official-support.md 的逐客户端支持矩阵与原生/反代选择，再实现 CommandCode 发送路径。
- 独立配置可加载、校验、覆盖和初始化，注册/路由可选择 CommandCode。
- httptest 合成上游覆盖流式/非流式、工具调用、缓存 usage、错误与取消；不使用真实 API key。
- 与客户端相关的请求/事件合同通过测试，文档不超出证据。

## Validation

`go test -race ./internal/provider ./internal/handlers ./internal/config ./internal/transformer ./cmd/routatic-proxy`
