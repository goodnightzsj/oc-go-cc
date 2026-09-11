# 新版控制台线上验收

## 结果

2026-09-12 00:05–00:09（Asia/Shanghai），通过现有Edge标签页验收部署b3c947d、UI e12b7231828e。没有新开浏览器、导出凭证、插入业务数据、保存配置或触发付费请求。

| 检查 | 结果 |
| --- | --- |
| 自动断言 | 563通过、0失败 |
| 数据范围 | 29个平台/页面组合，五个平台独立查询 |
| 布局 | 70组，七页、中英文、明暗主题、1440/768/390/320px |
| 只读请求 | 468，0意外请求、0页面异常 |
| 交互 | 键盘导航、前进后退、排序、焦点、高级筛选重置、配置定位与可访问名称通过 |
| 人工视觉复核 | 七页桌面/手机及主要深色页面；保存21张脱敏截图 |
| 服务 | active/running，NRestarts=0，health=ok |
| 数据完整性 | quick_check=ok，requests=0，provider_usage=1390 |

原始结果：`/tmp/oc-go-cc-console-final.8FPZgZ/edge-production-resume/console-verification.json`；同目录有脱敏截图。复验脚本：[verify-console.mjs](verify-console.mjs)，须提供现有edge-debug-attach driver的`EDGE_CLIENT`及输出目录`CONSOLE_EVIDENCE_DIR`，不自行启动浏览器。

## 账户与验证边界

- CommandCode：点数、订阅、用量三块真实账户数据均可用；月度剩余为70美元计价用量点数，不等同现金余额。缺月度总额度时不推算月百分比。
- Go：官方账户查询仍失败；本地展示表为空与该认证失败是两个独立问题。
- Zen：无公开账户查询合同；AWS：账单查询未启用；OpenRouter：未配置。保留父任务#8，不以UI适配代替账户授权。
- 生产请求表为空，因此线上证明的是实际空态/错误态/账户数据与交互，非空历史和多平台数值语义由此前隔离回归覆盖；没有往生产填入假记录。没有实测Safari/Firefox或其它系统实机。

## 失败记录与收尾

第一次脚本遇到CDP无返回值被写为`undefined`；第二次在45项检查后driver退出/超时。两次都未计通过。无返回值统一为`null`，清除自己遗留的监控与命令，重新附加同一Edge后才得到本次通过结果。

浏览器原语言、主题、筛选与页面状态已恢复，临时监控/遮罩均移除；CDP连接正常关闭。浏览器调试端口需由用户在inspect页关闭。

发布前660顶层/1094含子测试的race记录、vet、六目标构建与显式Codex双轮测试见`/tmp/oc-go-cc-console-final.8FPZgZ/`；本次源码SHA256复核一致，不把旧测试说成本次重跑。
