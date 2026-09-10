# Chromium 面板验收

- 日期：2026-09-10（UTC）；Chromium `152.0.7977.83`。
- 执行：Playwright `browser_run_code_unsafe` 加载同目录 `browser-smoke.js`，退出成功。
- 使用隔离 browser context、本项目静态页面和合成 `/api/*` 响应；外网请求全部拦截。没有访问真实配置、账单、账户或代理服务。
- 三种视口：1440×900、768×1024、390×844。每种视口均切换 overview/history/performance/fallback/analytics/quota/settings；21 个检查的 `page == viewport == panel == panelWidth`。
- 交互：五平台历史切换至 CommandCode 后从 5 行变为 1 行；套餐平台键盘选择成功；CommandCode 显示未获取并保留官方入口，隐藏 Go 数据且主动刷新不请求 Go 额度。
- 保存结果恰为 `{"commandcode":{"timeout_ms":1500,"zero_data_retention":true}}`，不发送未改字段或密钥掩码。
- 概览保留未知费用 `+ ?`；无未捕获脚本错误、无意外 API/外网请求。
- 截图已人工检查：`/tmp/oc-go-cc-dashboard-desktop.png`、`/tmp/oc-go-cc-dashboard-mobile-quota.png`。
- 限制：这是 Chromium 桌面引擎的窄屏验证，不是手机/平板实机、Safari/Firefox、触摸硬件、真实后台或所有系统验证；截图使用合成数据。
