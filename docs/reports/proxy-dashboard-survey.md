# 反代项目面板调研:数据展示横评

## 范围与方法

目的:找出其他 LLM 反代/网关项目的**面板数据展示**做法,识别本项目可借鉴项与已领先项。

方法:读各项目 README 与仓库内实际截图。**证据分级标注** —— 标注「截图实证」的是我下载并实际查看过的面板截图;标注「README 描述」的只有文字,没有可核对的面板图。

本轮**没有**逐项目读前端源码,所以下文是「他们展示了什么」,不是「他们代码里怎么实现的」。凡涉及动手实现,须先读对方源码再落。

调研时间:2026-09-22。

## 覆盖与证据质量

| 项目 | 形态 | 面板证据 | 证据等级 |
|---|---|---|---|
| `seakee/CPA-Manager`（即 cpa） | CLI Proxy API 的单文件 React 管理面板 | 5 张截图,实际查看 2 张 | **截图实证(最强)** |
| `bestruirui/octopus` | Go 聚合网关,React 面板 | 10 张桌面+移动截图,实际查看 3 张 | **截图实证** |
| `chenyme/grok2api` | Go 网关,内置 React 管理端 | 9936×2538 多页拼接图,实际查看 | **截图实证** |
| `songquanpeng/one-api` | 经典 LLM 分发系统 | 2 张截图,实际查看 | **截图实证** |
| `jianshuo/ccglass` | 本地日志反代 + web 面板 | 仅 demo.gif,未逐帧 | README 描述 |
| `QuantumNous/new-api` | one-api 二开 | README 无面板图 | 文字 + issue #5432 |
| `Wei-Shaw/sub2api` | Go 网关,React 管理端(CRS 2.0) | README 只有合作方 logo | README 描述 |
| `Wei-Shaw/claude-relay-service` | Claude 中转,多账户 | README 无面板图 | README 描述 |
| `router-for-me/CLIProxyAPI` | 多 CLI 协议网关 | README 只有赞助商 logo | README 描述 |
| `AIDotNet/ClaudeCodeProxy` | 代理管理系统 | 未取图 | README 描述 |
| `getmaxim/bifrost` | 商业 LLM 网关 | 官方状态页 | **实测页面** |

`octups` 未定位到确切项目,`bestruirui/octopus` 疑似即为此。X 检索只返回 5 条裸链接,**正文为空,未采信**,不作为证据。`claude-relay-service` 的 README 顶部是一条安全通告:v1.1.248 及以下存在管理员认证绕过,攻击者可未授权访问管理面板,已迁移到 sub2api。

## 逐项目发现

### CPA-Manager(证据最强)

这是横评里数据密度最高的面板,也是与本项目最可比的一个(同样是「本地反代 + 观测面板」)。

**实时监控页**:
- 顶部统计卡带**趋势副标**:Requests Received `1,082 (12/min)`,Total Tokens `7.2M (in 6.9M / out 324.1k)`,Total Cost `$62.84 (avg $0.058/req)` —— 主值下面直接给速率与均值,不用另开页
- 筛选栏:时间范围 `Last 1 Hour` / 平台(带徽章计数)`All Platforms` / `All Models` / `All Providers` / `Reset`
- **每秒聚合的时序表**:列为 Request ID / Timestamp(精确到秒)/ Source(平台徽章)/ Model / Provider(带 logo)/ Status(Success 徽章 + 200)/ Duration(秒)/ Tokens(in/out)/ Cost
- 有 Live 指示 + Pause 按钮 + 显示计数 `Showing 50 of 1,082 requests` + 每页条数切换
- 副标行给出**平均延迟 1.05s** 与**成功率 100.0%**

**账号总览页(表格模式)**:
- 顶部三张聚合卡:Statistic(账号数、Pro 数、Windows 账号数)、API 探针(探针数、成功、失败)、限额使用(主窗口额度用量 66%)
- 一张大表,行 = 账号,列 = **Status / Account / Req / Success rate / Avg. latency / Primary window(额度百分比 + 进度条)/ 7d usage / 5h usage / 上次探测 / 操作**
- 账号列同时给:邮箱、`#user_id`、`acct_id`、平台徽章、套餐徽章、订阅到期日
- **主额度窗口**同时给重置倒计时(`3h 26m left`)和窗口大小(`5h`)
- 表下有 `Showing 1 of 1 accounts · Auto-refresh 10s · Last refreshed 02:05:44 AM`
- 支持**表格模式 / 卡片模式**切换 —— 卡片模式每账号一块,显示健康指标、token 用量、Codex 额度、Top 2 模型明细

### octopus(证据强)

**Dashboard 页**:顶部一排等宽统计卡(Total Requests / Total Tokens / RPM / TPM),每张卡左上角一个图标 + 标签,数值字号很大,卡片下方是两列等宽图表区(Requests Over Time 折线、Top Models 横向条形)。

**Logs 页**:这是最值得看的一页。
- 每个模型一块可折叠卡片,标题行 = 模型名 + 该模型的 `price`(输入/输出单价)+ `token usage` 汇总
- 卡内是请求明细表,列为:状态列(色块图标)、时间、输入/输出 token、耗时
- 有独立的「Request Details」抽屉页,把单次请求拆成四个信息块:**Model & Price**(模型、输入输出单价、耗时、起止时间)、**Request**(ID、时间戳、耗时、状态码)、**Token/Pricing Breakdown**(输入/输出/缓存命中/缓存创建 token + 单价)、**Stats**(总计与缓存率百分比)

这个「单请求详情 = 价格 × 用量 × 结算」的四块结构,与本项目 Cost Breakdown 弹窗目标一致。

**Prices 页**:一张大表,列 = 模型 / 输入价格(默认) / 输入价格(自定义) / 输出价格(默认) / 输出价格(自定义) / 更新时间。**默认价与自定义价并排两列**,一眼看出哪些被覆盖。

### grok2api(证据强)

- 顶部 5 张统计卡:Total Requests、Total Tokens、Active Accounts、Avg. Response
- 「Request Breakdown」表:列为请求数 / 原始 token(输入·输出)/ 计费 token(输入·输出)/ 缓存读 / 计费缓存读 —— **原始 vs 计费并排**
- 「Token Distribution」带表头的横向条形
- 「Model Token Usage」按模型分块
- Accounts 页账号卡信息密度很高:每张卡显示 provider(带 logo)、账号 ID、**额度百分比进度条**、状态徽章(正常/限流)、按文本/图片/视频分类的剩余额度、三项风控统计(采样数/成功率/平均耗时)
- Accounts 页顶部另有一排**风控聚合卡**:Control 策略(采样 N / 保留 N / 限流 N)、Sampling Stats(样本数/成功/失败/成功率)、Protection(风险账号数)
- 每张账号卡支持批量勾选 + 筛选水印

### one-api(证据强,但是反面参照)

面板是**纯管理 CRUD 表格**:渠道列表 = 名称/状态点/类型徽章/分组/已用额度/优先级/操作列;令牌列表 = 名称/状态/已用额度/剩余额度/创建时间/操作。没有图表、没有时间序列、没有分布。它的定位是「分发与计费管理」,不是「运行观测」。

### new-api

README 无面板图。issue #5432 是一份很具体的需求陈述,其诉求本身就是一份「数据看板该有什么」的清单:

- 聚合维度:按渠道 / 令牌 / 模型 / 日期,以及两两交叉(渠道×令牌、渠道×模型、令牌×模型)
- 筛选:渠道、令牌、模型、分组、成功/失败状态
- 统计字段最小集:调用次数、成功次数、失败次数、输入/输出/总 token、扣费额度、平均响应时间、**平均吞吐量(tokens/s)**、成功率
- 展示:顶部总览卡 + 可排序聚合表 + CSV 导出;时间快捷档(今天/昨天/7天/本周/30天/本月/全部)+ 自定义区间
- **口径一致性诉求**:要求统计页与后台看板的总览口径一致;若由日志明细聚合,必须显式说明统计范围(是否含失败日志、模型测试日志、缓存 token、异常请求)

### sub2api

README 描述:Go 网关 + 内置 React 管理端,运维面覆盖 Dashboard、模型路由、客户端密钥、审计、运行设置。模型路由页展示 Provider 前缀、接口能力、支持账号数。账号操作(批量导入导出、额度同步、凭据续期)均显示实时进度。审计记录输入总量与缓存部分用于对账,`audit.ledgerMode` 可取 `observe` 或 `enforce`(后者为保计费准确会暂停新推理)。

### bifrost(实测页面,反面参照)

官方状态页:每平台显示状态标签 + 事件数 + 组件比率(如 `24 / 24 up`)。

**它自己打自己**:顶部写 `Active Incidents 2`,下方过滤页签写 `Issues (1)` —— 两个数不同源。而且**没有延迟数字、没有可用率百分比、没有 sparkline**。说明「少即是好」不成立,该有的量化不能省。

## 本项目已领先的部分

调研后确认,以下几项在横评中不属于落后项:

| 能力 | 本项目 | 对照 |
|---|---|---|
| 双主题 | 暗/亮均实测通过,硬编码色有重映射层 | octopus 仅暗色 |
| 图表 | 原生 SVG 折线 + 图例切换 + 方向键游标 + 无障碍描述 | one-api 无图表 |
| 成本口径 | 区分「已知费用」与「价格未知」,不编造 | 多数项目直接显示一个数 |
| 平台隔离 | 同名模型跨平台不串账 | grok2api 无多平台维度 |
| 移动端 | 表格在滚动容器内,页面无横溢 | 少数项目有移动截图 |

## 可借鉴清单(按价值/成本排序)

| # | 来源 | 做法 | 本项目现状 | 成本 | 价值 |
|---|---|---|---|---|---|
| 1 | CPA-Manager | **每行同时给「成功率 + 平均延迟 + 主额度窗口倒计时」** —— 平台健康一眼可判 | 概览只给全局成功率,无平台级健康行 | 中 | **高** |
| 2 | CPA-Manager | **统计卡主值下挂趋势副标**:`$62.84 (avg $0.058/req)`、`1,082 (12/min)` | 有 metric-note 但未接速率/均值 | **低** | **高** |
| 3 | 本项目已有 | `circuit_breakers` 已在 `/health` 输出,面板未用 | 只差 UI | **低** | 高 |
| 4 | octopus | **单请求详情四块结构**:Model&Price / Request / Token 明细 / Stats | 已有 Cost Breakdown 弹窗,可对照补齐 | 低 | 高 |
| 5 | 本项目已有 | `fallback_rate` 后端已算并有测试,UI 零引用 | 只差 UI | **低** | 中高 |
| 6 | octopus | **默认价 / 自定义价并排两列**的价目表 | 价格页无此对照 | 低 | 中高 |
| 7 | CPA-Manager / new-api | **列表尾部给「显示 N / 总数」+ 自动刷新周期 + 上次刷新时刻** | 有分页但未显式声明口径 | 低 | 中高 |
| 8 | new-api #5432 | 平均吞吐量(tokens/s);本项目只有总耗时,无 TTFT | 缺 | 低 | 中高 |
| 9 | CPA-Manager | **表格模式 / 卡片模式切换**(同数据两种密度) | 无 | 中 | 中 |
| 10 | grok2api | **原始 token / 计费 token 并排** | 只有最终计费口径 | 低 | 中 |
| 11 | octopus / grok2api | 每模型一块的可折叠日志卡 + 模型级 price/usage 汇总头 | 现有按请求平铺 | 中 | 中 |
| 12 | CPA-Manager | 筛选器带**计数徽章**(`All Platforms (3)`)+ Reset | 有筛选无计数 | 低 | 中 |
| 13 | new-api #5432 | 二维交叉聚合(渠道×模型) | 只有单维 | 中 | 中 |
| 14 | octopus | 价目表「更新时间」列(暴露价格新鲜度) | 有小时级刷新但不可见 | 低 | 中 |
| 15 | 本项目审计 | 字号阶梯 16 级收成 8 级(9 个尺寸写成两种拼法) | — | 低 | 低(卫生) |

## 两条必须守住的边界

1. **口径一致性**(来自 new-api #5432 的用户诉求,且 bifrost 正在犯):同页面的不同数字必须同源。本项目已有「实例账本 vs 平台官方」的区分,任何新增聚合都必须标明范围。
2. **不抹掉已知未知**(本项目既有约束):价格未知的请求报 unknown,不估算成 0,也不并入已知费用。grok2api 的「原始 vs 计费并排」是加强这一点的好形式,而非削弱。

## 明确不做的

- **不引入图表库**。现有原生 SVG 已覆盖折线、分布条、进度环;octopus 用 ECharts,但我们的需求规模不需要。
- **不做会话级聚合**。ccglass 的 session 维度依赖其自有会话标识;本项目历史按请求记录,引入会话维度需要上游传会话 id,属于协议层改动,不在面板范围内。
- **不做 Trace 瀑布**。langfuse 式嵌套 trace 面向多步 agent 链路;本项目是单跳代理,没有嵌套结构可展开。
