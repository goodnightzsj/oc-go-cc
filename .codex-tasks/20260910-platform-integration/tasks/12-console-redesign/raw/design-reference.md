# 七页重设计参考与决策

2026-09-11通过MySearch和GitHub官方源码核实。仅借鉴信息架构，独立实现原生HTML/CSS/JS，不复制第三方组件、图标资产或后端。

- [sub2api AppSidebar](https://github.com/Wei-Shaw/sub2api/blob/main/frontend/src/components/layout/AppSidebar.vue)：桌面持久侧栏、分组入口、移动遮罩和折叠；本项目保留七个既有入口，不引入其账户/用户/支付功能。
- [sub2api DashboardView](https://github.com/Wei-Shaw/sub2api/blob/main/frontend/src/views/admin/DashboardView.vue)：概览数据分组，统一时间筛选，并列模型分布/Token趋势和近期用量；本项目不引入Chart.js或合成趋势。
- [new-api authenticated-layout](https://github.com/QuantumNous/new-api/blob/main/web/src/components/layout/components/authenticated-layout.tsx)：全局页头、侧栏、独立内容容器；本项目复用hash页签和现有控件，不引入React或身份体系。

默认方向：sub2api式清楚的运营层级，结合new-api式控制台框架。浅色为清洁的灰蓝底/明亮表面，深色用分层石板色；主色克制靛蓝；系统/CJK字体免外部加载，数字对齐，按钮与文字分别有明确层级。

七页：概览四主指标/次级运行条/趋势；历史主筛选/高级筛选/紧凑汇总/请求表；性能统一工具条/样本说明/比较表；降级策略主路由/编号链/操作区；分析主指标/趋势/日期明细/分布；套餐官方点数/窗口/订阅并列、本地账本单独区；设置平台分组/高级项/可达保存区。

真实空态必须说清“保留的本实例记录为空”，不能说账户从未消费。Go旧数据诊断见docs/go-history-diagnosis.md。恢复旧记录和变更retention不包含在纯UI改造中。
