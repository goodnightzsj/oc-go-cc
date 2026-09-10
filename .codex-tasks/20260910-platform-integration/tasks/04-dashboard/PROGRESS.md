# Progress

## Context Recovery Block

- 当前：13/13；AWS独立配置、SDK账单、页面和五平台有数据/空数据矩阵通过；真实账户条件仍保留在#8。
- 真源：TODO.csv；主线程负责后端、页面、验证和整合，所有旧代理写入权限失效。
- 下一步：#6重新部署并验收；真实账户权限或未公开API不能被合成验证替代。
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

## AWS 增量恢复核验

- 新增未提交AWS代码保留；隔离HOME定向 `go test -race -p 2 ./internal/config ./internal/gui ./internal/quota -run BedrockBilling -count=1` 通过。配置默认关闭、独立账户scope与部分更新均验证。
- 全包初轮保留两项失败：原平台能力和DOM测试仍断言AWS只有占位；下一步按新增官方账单合同扩展，不删除跨平台隔离断言。
- 新 `TestBedrockBillingPaidQueryUsesPost` 先失败：GET查询参数可以产生收费请求，POST不支持。修复收费动作为显式POST并使用Go标准库CrossOriginProtection，普通GET/refresh不触发收费查询。
- 当前工具子代理发生环境解码错误，无有效独立结论，不计审查通过；主线程继续合同核验和验收。上游HEAD复查仍为b214eeb，无新增提交。

## 本次恢复与发布前复核

- 保留全部未提交 AWS 改动，从活动 CSV 第11项继续；完整复核账单查询、配置、页面和测试链路，没有重复实现已完成的其他平台功能。
- 最新隔离 HOME 定向全包 `go test -race -p 2 ./internal/config ./internal/gui ./internal/quota -count=1` 退出0：2.740s / 2.673s / 1.319s。SDK 环境/profile优先级、SigV4、固定端点、单账户过滤、分页失败、币种、负/零/未知值、POST与跨站拒绝、局部配置保存和过期响应隔离均通过。
- `node --check internal/gui/assets/app.js`、`git diff --check` 通过；CommandCode/Zen/AWS官网再核对一致，Codex手册确认当前，upstream HEAD仍为b214eeb。
- page_gate发生工具解码错误；旧backend_audit恢复消息未回传后停止。两者均不计为独立审查通过；主线程承担审查与验收。后续证据目录 `/tmp/oc-go-cc-multiplatform-final.FHESMh/`。
- 最新Chrome152有数据矩阵459检查、31布局、261请求、6次独立保存、1次合成AWS手动POST全部通过；普通刷新/平台切换没有产生额外收费动作。空账本210检查、29平台/页面组合、21布局通过；两轮均无脚本错误或意外请求。结果为上述目录的browser-result.json与empty-browser/result.json。
- 首轮有数据浏览器矩阵已通过，但主线程延后关闭fixture导致Go测试宿主10分钟超时；随后stop请求连接拒绝。空账本fixture明确POST停止并正常PASS（550.33s）。这是验证清理遗漏，保留失败、不归因产品错误；运行器改为成功确认十条合成记录后立即停止fixture，重跑验证生命周期。
- 修正运行器后矩阵再次459检查/31布局通过，1次合成AWS手动POST，259请求；fixture53.23s自动退出PASS（包54.668s），没有增加超时或改产品代码。原失败和两份browser结果均保留，最新为browser-cleanup-result.json。
