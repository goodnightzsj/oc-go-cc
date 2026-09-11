# Progress

## Context Recovery Block

- 当前：4/4，上一轮实现与线上 Edge 验收完成；用户仍不满意呈现方式，新增 #12 全页重设计。
- 真源：TODO.csv、docs/uiux-multiplatform-review.md、.impeccable.md。
- 下一步：#11 分析 Go 旧数据消失，再由 #12 按 sub2api / new-api 风格重设计；当前 Edge driver 保持连接。
- 验证：`TestUIUXEditingBehavior` PASS；`TestPlatformQuotaBehavior` 因旧费用文案断言失败（真实新值 Known $0.250），待按已授权的新文案更新；账户测试在旧占位分支失败。原始结果 `/tmp/oc-go-cc-uiux-resume.xi9Muc/initial-targeted.jsonl`。

## 当前验证与部署准备

- 上述旧文案及占位失败已修正；最新全量race656顶层/1090含子测试、vet、六目标CGO0构建通过。证据/tmp/oc-go-cc-uiux-verify.NqKMYt/；未将默认关闭的CLI/浏览器fixture算作实测。
- 用户明确改用edge-debug-attach调试远端；下载进程已取消，合成服务已退出。现有Edge连接保持，目标面板仍运行0b28ab2；未读取真实凭证、保存生产表单或触发收费查询。
- 发布前备份/root/oc-go-cc/.tmp/predeploy-20260911-commandcode-F3MPti0m，权限700；SQLite quick_check=ok、requests0、provider_usage1390，旧release路径已保留。
- CSV校验第一次因macOS Ruby默认US-ASCII报错；明确指定UTF-8后11份CSV均通过。源码JS语法与git diff --check通过。

## 当前 Edge 验收与新增需求

- 远端 e29e79f，release20260911161610-0fac00358316，服务 active/running、NRestarts0；UI 01d300b9dadf。仅浏览既有已登录 Edge，没有再次部署或保存生产测试配置。
- 29平台/页面组合与70布局（七页、中英、浅深、320/390/768/1440）通过；首轮494检查中487通过，7项误判保留在 remote-verification.json。根因为CDP Enter缺少字符、焦点仅检查outline而漏box-shadow、名称未解析aria-labelledby；keyboard-probe.json的7项真实按键/AX树复核全通过。产品源码未因此修改。
- 浏览器原始证据 /tmp/oc-go-cc-edge-remote.Peo95y/；确认本地历史/统计实际为空，不能将空集合筛选检查冒充非空数据归属验收。该问题由新增#11追查。
- 发现P3：中文历史分页仍为Prev/Next、设置Server/Logging组未翻译。用户要求全页重新设计，随#12统一处理，不将此次工程验收描述为视觉最终满意。
