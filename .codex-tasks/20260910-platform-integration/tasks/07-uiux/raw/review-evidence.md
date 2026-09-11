# UI/UX 观察证据摘要

2026-09-11，源码0b28ab2，UI build95403ba4a0a5。原始目录：`/tmp/oc-go-cc-uiux-current.axtgQT/`；观察脚本为同目录任务工件`observe-uiux.cjs`。

- `ROUTATIC_BROWSER_SMOKE=1 go test ./internal/gui -run '^TestMultiPlatformBrowserServer$' -count=1 -v`：PASS，测试64.49s，包64.905s，使用隔离HOME/SQLite和合成上游。
- `node .../observe-uiux.cjs http://127.0.0.1:63562 /Users/zsj/.npm/_npx/2334a3ea0ef73d73/node_modules/playwright /tmp/oc-go-cc-uiux-current.axtgQT`：退出0，14页视图、5套餐视图、0pageerror、0blocked，随后仅停止已验证身份的合成fixture。
- 1440/390px均为900px高；面板clientHeight809px。历史表offset605/1358px；设置53个展开字段，scrollHeight3782/4087px；分析和套餐表头最小9px。
- 历史/性能14个排序表头均tabIndex=-1且无button。性能Avg：Enter后aria-sort仍none；click后descending。
- 四个备用链条目draggable=true、tabIndex=-1，无上移/下移按钮；跨平台同名模型删除按钮均为Remove shared-model。
- 代理、自启动、通知三个设置checkbox无可访问名称；图表方向键能移动，但aria-valuetext仅日期。
- 深浅系统media分别true/false，body背景和文字值均未改变：rgb(17,19,21)、rgb(243,244,246)。中文html.lang正确，部分旧设置行为文案仍英文。
- 截图和ARIA包含的模型、Key提示和金额全部为合成数据。尺寸扫描中隐藏的原生select源节点不作为触摸缺陷；未宣称屏幕阅读器或WCAG完整认证。
