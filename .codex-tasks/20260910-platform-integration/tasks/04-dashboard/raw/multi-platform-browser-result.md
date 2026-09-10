# 五平台真实后端浏览器验收

日期：2026-09-11。Chrome 152.0.7977.83 + 已安装 Playwright 1.61.1，临时 profile/context；使用 `TestMultiPlatformBrowserServer` 的真实 Go GUI 与 SQLite，全部账户与上游均为合成值。

## 结果

- 最终脚本 `multi-platform-browser-smoke.js`：447项检查通过，249个同源请求，0未捕获脚本错误、0越界请求。
- Overview、History、Performance、Analytics：五平台及All逐项切换；同名模型、请求数、缓存合计、未知费用小计与趋势范围一致。
- Quota：五平台独立本地数据；Go窗口；OpenRouter一Key成功/另一Key401、BYOK独立、负余额；Zen/CommandCode/AWS能力缺口均按真实状态展示。
- Settings：五次仅修改各自timeout的POST，完整配置前后对比确保其他显式字段不变，管理Key始终脱敏。
- Analytics合成503显示错误与未知值，随后从真实API恢复；不显示伪造的零。
- 1440/768/390宽度×七页面，加390/320宽度×五平台套餐页，共31项布局检查无面板或页面横向溢出。
- 截图：`/tmp/oc-go-cc-multiplatform-1789073459882-6420034bbee4f/desktop-{overview,history,performance,fallback,analytics,quota,settings}.png`。

## 保留的首次失败与修复

1. MCP执行沙箱没有URL、alpha Playwright缺浏览器：环境问题。改用现有稳定Playwright和已安装Chrome，不安装依赖、不碰日常浏览器。
2. 第一轮372检查后发现测试fixture未设置stream timer，修改timeout依法联动默认值，whole-config断言失败。实证只变化timeout与其派生值；固定fixture独立timer，不改产品默认行为。截图 `...1789072717621-5a1ff1e78ceea/first-failure.png`。
3. 第二轮414检查时复现真实390px套餐页溢出，面板宽455px。标题/端点与筛选控件横排造成；在既有620px断点改为纵向排布。截图 `/tmp/oc-go-cc-multiplatform-1789073011590-915efd9a736d1/first-failure.png`。
4. 修复后的完整矩阵与额外320px回归通过。三次fixture均通过专用stop入口清理。

## 边界

这些是实际浏览器访问真实本地服务的合成合同测试，不是五个平台真实账户生成/扣费/余额验收。其它浏览器、真实iOS/Android设备和Windows/Linux桌面运行仍未覆盖。无公开合同或授权的账户能力仍保留为缺口，不能称为账户余额全部接入。
