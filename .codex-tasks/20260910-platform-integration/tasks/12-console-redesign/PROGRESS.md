# Progress

## Context Recovery Block

- 形态single-full；3/5；当前第4步全量验证与安全部署；#11已有诊断与计数证据，禁止顺手恢复账本或改变retention。
- 用户参考：sub2api / new-api；保留原生技术栈。主线程重新完整读取设计技能、相关参考、HTML/CSS和任务真源，.impeccable.md含此次设计要求。
- 官网参考已记录raw/design-reference.md；固定桌面侧栏+顶栏，移动横向导航；日志主/高级筛选，官方账户/订阅/本地账本分组。
- 当前基线e29e79f，Edge原连接仍存活并确认UI01d300b9dadf；只用现有/tmp/oc-go-cc-edge-remote.Peo95y/client.mjs控制该连接。
- 写入由主线程统一接手；protocol_gate/backend_audit已中断，未取得有效独立结论。console_regression_review因模型部署路由错误退出，不计作审查通过。
- 验证：原Edge连接仍存活（PID36262），线上UI01d300b9dadf；当前JS语法和diff检查通过。首轮GUI回归仅TestPlatformQuotaBehavior失败：旧断言要求原始平台ID，新展示为可读平台名；补充可读名称与data-provider双重断言，不改变后台归属。

## 2026-09-11 接续验收

- 完整核对当前HTML、JS及相关DOM回归；保留已有布局实施，不重做已完成#11诊断。
- MySearch重新取得sub2api DashboardView和new-api authenticated-layout一手源码；只借鉴导航与数据分组，不移植框架、品牌、后端或组件代码。
- 隔离HOME首轮定向与GUI回归证据：/tmp/oc-go-cc-console-check.x4nUqW/targeted.jsonl、gui-baseline.jsonl；两次均同一过时平台标签断言失败。新Console结构/保留数据空态、CommandCode账户和UIUX编辑行为通过；不作为最终全量通过。

## 最新验证与发布准备

- 复用原Edge连接确认远端仍为UI01d300b9dadf，未新开浏览器；新的只读SSH计数仍为requests0/provider_usage1390，服务active/running、NRestarts0。
- 主线程完整核对HTML/CSS及JS差异和关键调用方；GUI/quota隔离race通过（6.639s/3.747s），证据/tmp/oc-go-cc-console-final.8FPZgZ/gui-initial.log。随后新增ThemeBehavior，Console/UIUX/PlatformQuota定向通过（1.357s）。
- 新增raw/verify-console.mjs复用Edge driver，白名单只读网络、七页五平台、双语双主题、键盘和窄屏检查；不导出凭证、不触发付费查询。初次脚本语法检查发现一个多余右括号，修后node --check通过；尚未宣称线上验收通过。
- FastCtx本轮输出池耗尽后按规则使用sed/rg，未绕过权限或读取任务外数据。
- 下一步：重新生成静态样式、最新全量race/vet/六目标构建；备份后按原脚本部署，再运行同一Edge的线上验收。

## 发布门禁通过

- 当前源码全量race退出0：660顶层、1094含子测试pass，18个有测试包；vet退出0，darwin/linux/windows×amd64/arm64六个CGO=0目标构建及二进制架构检查通过。源码/资源前后SHA256核验一致。
- TestCodexResponsesCLIToolRoundTrip显式运行通过（2.62s）：两轮合成推理、工具往返和两条隔离CommandCode记录；未调用真实付费账户。TestMultiPlatformBrowserServer依设计默认关闭，生产验收仅用原Edge。
- 本机无Tailwind编译器；离线检测失败后，在/tmp/oc-go-cc-console-final.8FPZgZ/build-tools安装已声明3.4.19（禁安装脚本），生成compiled-tailwind.css通过。npm首次隔离配置路径重用报错，改为独立路径后通过；Browserslist旧数据库警告保留，不升级依赖。
- 全量证据：/tmp/oc-go-cc-console-final.8FPZgZ/{race.jsonl,vet.log,build.log,codex-smoke.jsonl,sources-before.sha256,sources-check.log}。
- 已在远端创建0700备份/root/oc-go-cc/.tmp/predeploy-20260911-console-4V8icArJ，配置与SQLite快照文件0600；备份quick_check=ok，requests0/provider_usage1390。未恢复或修改原账本/配置。
- origin/main与本地HEAD同步；远端只存在既有.ace-tool未跟踪目录。下一步提交推送及标准部署，保留全部release，之后原Edge真实验收。
