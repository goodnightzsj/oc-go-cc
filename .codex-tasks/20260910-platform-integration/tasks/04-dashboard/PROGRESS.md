# Progress

## Context Recovery Block

- 当前：9/9；部署边界修复完成，最新全量641顶层/1039含子测试race与Chrome447断言/31布局重新通过；等待重新部署验收。
- 真源：TODO.csv；主线程负责后端/config/整合，platform_ui 限定写入页面和对应行为测试。
- 下一步：父任务 #5 完成文档/构建/部署门槛；真实账户权限或未公开API不能被合成验证替代。
- 约束：无真实凭证读取；入口与未知状态不能作为真实套餐接入完成。未提供的能力须明确证据和剩余限制。

## 验收证据

- 最终全量 `go test -race -p 2 ./... -count=1` 通过；GUI测试在上海/洛杉矶双时区覆盖小额、零、未知成本、Top-N分母、UTC桶、平台归属与配置保留。
- 真实 Chromium 152.0.7977.83 测试三视口1440/768/390、七页签，21项布局无页面溢出；键盘平台选择、历史筛选、两字段PATCH、CommandCode隐藏Go额度及官方入口通过，无脚本错误或意外网络。详见 raw/browser-smoke-result.md。
- 主线程已人工检查桌面与移动截图，关闭本轮临时静态服务；未操作用户浏览器登录态、真实后台或全局客户端配置。

## 2026-09-11 新一轮工作

- Overview/Performance/Analytics 的五平台/all 筛选与请求序号隔离已实现；History 在筛选变更时清掉旧平台行。平台筛选行为测试通过，整个 GUI 的旧源码字符串断言正按新行为合同更新。
- 后端 `requestedProvider` 显式校验，`storage.Window.ForProvider` 与 `Latency.ForProvider` 保持汇总、趋势、比较、同名模型统计范围一致；合成数据包含五平台和未知历史行。
- OpenRouter quota 解析器已有合成测试并通过；新增 GUI/config 集成回归先失败后补实现，管理 Key 不进入推理池、不回退到普通 Key。账户 Key 限额、BYOK 与 Credits 单独返回；缓存按平台保存且管理 Key 改动立即失效。
- Zen/CommandCode/AWS 的账户查询缺口用 `status/source/reason` 表达，五平台本地账本仍用同一 analytics 真源独立获取。真实账户未请求；套餐私有页面未抓取。
- 子代理两次同型工具解码失败，已停止重复尝试并由主线程接手；不是源码或测试失败。
- 最新主线程定向 race：`TestDashboardProviderScope`、`TestPlatformPerformanceWithoutStorageIsExplicit`、OpenRouter 配置/GUI/多 key/cache 与五平台 capability 用例通过（config 1.189s、gui 1.549s）。`go test -race ./internal/quota -count=1` 通过（1.429s）。
- 额外失败→修复证据：Go 旧 quota 会跟随重定向把 Key 送往同主机不同应用，502 body 会回显 Key，且 UsageURL 接受 userinfo/file URL。已改为显式拒绝这些 URL、拒绝重定向、只报告状态码并脱敏解析错误；合成安全回归通过。

## 当前收尾验证

- CSV 导出分页时会重新读取当前筛选，切换平台后第二页混入新平台。`TestHistoryCSVExportKeepsInitialQuery` 501 条两页异步复现先失败；固定初始查询后与平台/套餐行为定向 race 通过（1.638s）。
- 主线程清洁环境定向 `go test -race -p 2 ./internal/config ./internal/gui ./internal/quota -count=1` 全通过（2.726s / 2.514s / 1.337s）。
- `TestMultiPlatformBrowserServer` 新增清除继承的 ROUTATIC_PROXY_/OC_GO_CC_ 覆盖项，HOME、配置、DB、上游均隔离；浏览器禁止系统自启写接口和外网。尚未将浏览器记为通过。

## 浏览器首轮与测试合同校正

- Playwright MCP 执行沙箱没有 `URL`，现有 alpha 包对应浏览器未下载；不安装依赖，改用已安装的 Playwright 1.61.1 与 Chrome channel、新临时 profile 执行同一脚本。
- 首轮执行了372项断言，Overview/History/Performance/Analytics/Quota的所有平台与503失败恢复均通过；Settings第一项whole-config比较失败。
- 已核实实际变动只有 `opencode_go.timeout_ms` 与继承它的 `stream_timeout_ms`，符合 `internal/config/loader.go` 的默认规则。不是配置串改；fixture显式固定独立stream timers，保留原始完整配置对比，不修改产品默认行为。
- 首失败证据：`/tmp/oc-go-cc-multiplatform-1789072717621-5a1ff1e78ceea/first-failure.png`。旧fixture通过POST stop清理；新轮尚未验收。

## 最终浏览器验收

- 第二轮414项时复现390px套餐面板撑宽455px；调整现有620px CSS断点的套餐标题布局，未隐藏信息或更改数据。
- 最后一轮447项、31布局检查、249请求全部通过；含320px五平台套餐、五平台独立保存、部分401、负余额、503失败恢复，无脚本错误/越界请求。证据见raw/multi-platform-browser-result.md。
- 2026-09-11最新全量race退出0：639顶层/1029含子测试，18个有测试包通过；全量默认关闭的浏览器fixture已单独显式执行。真实账户、其它浏览器及OS实机未验收。

## 生产验收暴露的边界

- 首次生产只读冒烟在 Overview 第 8 项断言失败：空账本 `models=null`（进一步核对 `trend=null`），同接口的 providers/scenarios 已是空数组。不是跨平台串数；新客户端按数组消费仍有兼容性缺口。
- 合成 `TestOpenRouterQuotaDoesNotProbeGlobalKeys` 证明：只有全局 Go Key 时，浏览未配置的 OpenRouter 会向其默认账户端点尝试该 Key；只有管理 Key 时也会额外发送全局推理 Key。生产浏览器尚未到套餐请求，没有用真实 Key 复现。
- 新增五平台及 All 空集合回归、未配置/仅管理 Key 两种合成回归，均先失败。改动仅在共享查询结果初始化为空数组、OpenRouter 额度只取显式平台 Key；原有推理 Key 优先级及 Go 额度的全局 Key 兼容保持不变。
- API 与 OpenRouter 使用文档同步上述行为边界；既有 llmdoc 大范围架构同步仍待用户确认。
- 修复后 gui/storage/quota 全包 race 通过；完整 641 顶层/1039 含子测试通过。原 447 项浏览器矩阵在新源码上重跑成功，31 项布局、249 请求、无脚本错误/越界请求；证据 `/tmp/oc-go-cc-deploy-fix-verify.I3hRE1/browser-result.json`。
