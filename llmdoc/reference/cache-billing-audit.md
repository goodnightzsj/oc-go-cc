# 缓存与计费审计（2026-08-26）

记录一次跨三端的对账：OpenCode 官方账单、routatic-proxy 远端代理、本地 CompactGate 网关。核心结论：缓存 token 计数丢失 + seed 价格过时导致代理估算成本与真实账单偏差约 10-50 倍；两个缺陷均已修复并验证。

## 问题链

1. **缓存计数丢失**：OpenCode Go 的 oa-compat 网关在 OpenAI 兼容端点上把缓存拆分放在 `prompt_tokens_details.cached_tokens`（OpenAI 标准），而代理只解析 DeepSeek 风格 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`。`splitPromptTokens`（`internal/transformer/stream.go:645`）落入 "无缓存字段" 分支，把全量 prompt 记为输入。
2. **后果**：远端 `requests` 表缓存列全 0、输入列是全量（语义失真）；估计成本把 95%+ 的缓存 token 按输入价计费；面板 token 总量与成本均虚高。
3. **价格过时**：`internal/storage/seed_prices.json` 中 deepseek-v4-flash 记 0.14/0.28/0.0028，而 OpenCode 实际账单为 0.22/0.66/0.007（多行联立反推，误差 <0.1%）。

## 修复提交（均已部署远端）

| 提交 | 内容 |
|------|------|
| `de4af95` | debug_capture 接入 provider 流式路径（registry 路径此前从不执行 capture）；修复 `teeReadCloser` 管道写端永不关闭导致的 goroutine 泄漏与上游响应永不落盘（`internal/client/opencode.go` 的 `CaptureBody`） |
| `07802a1` | `CaptureEntry.Data` 从 `json.RawMessage` 改为 string：SSE 多文档流无法通过 RawMessage 的 MarshalJSON 校验，改前 upstream-response 写盘全部失败（`internal/debug/types.go`） |
| `d87554e` | 解析 OpenAI 标准 `prompt_tokens_details.cached_tokens`：`UsageInfo` 增加 `PromptTokensDetails`（`pkg/types/openai.go:136`）；`splitPromptTokens` 将其映射为 cache_read（input = prompt - cached）；`normalized_bridge.go` 合并两种风格 |
| `3f55918` | seed 价格更新为账单实测值（input 0.22 / output 0.66 / cache_read 0.007），测试断言同步 |

## 验证结果

- 修复后真实流：input 104349→74，cache_read 0→239616，成本 0.0148→0.000749，逐项数学吻合（74×0.22 + 239616×0.007 + …）。
- 价格更新后两条新记录与公式精确吻合到 6 位小数。
- 修复生效窗口（11:19 UTC 起）：代理 `cache_read_tokens` = CompactGate `cached_input_tokens` = 15,590,912，两边逐字节一致。

## 三端对账（2026-08-26）

| 侧 | 数据 | 窗口 | 说明 |
|----|------|------|------|
| OpenCode 账单 | usage 页 server-fn 数据 | 8-14、8-26 10:01:49 UTC 前 | `cost` 单位 = 1e-8 USD，即 `provider_usage.cost_units` |
| 远端代理 | `requests` 表 | 今天 09:59:21 UTC 起（回填/校验时 1147+ 行，持续增长） | input 存"全量"（修复前）/ "纯输入"（修复后），缓存单列；985 行 provider 级精确 |
| CompactGate | `request_logs`（compactgate-logs.sqlite） | 7-10 起，deepseek 今天 637 条 | `cached_input_tokens` 与代理 `cache_read_tokens` 同源 |

- 重叠锚点窗口（09:59:21–10:01:49 UTC）：OpenCode 账单 6 条 = $0.1496；代理估算 7 行 = $0.2624（虚高 1.75×，且全部来自缓存误计）。
- 全天总量：CompactGate input 127,652,539 vs 代理 127,508,956（差 0.1%，同一批请求）。
- **遗留**：账单部分行带 `costMultiplier: 2`（lite 计划倍率），代理按基础价估算，极端情况面板成本可能低于账单约一半；倍率触发条件在平台侧，代理无法还原。

## 相关配置

- `debug_capture`：`logging.debug_capture`（`internal/config/config.go:236`），默认关闭；当前远端开启且 `max_files=20`（≈1GB 上限），`redact_api_keys: true`；捕获文件在 capture 目录按 50MB 轮转。
- 部署方式：`ssh root@23.80.89.173` → `cd /root/oc-go-cc && git pull origin main && bash scripts/prod-deploy.sh`（release 目录 + 软链 current + systemd `oc-go-cc.service`）。

## 成本估算公式（当前） 

- cost = input×input价 + cache_creation×max(cache_write价, input价) + cache_read×cache_read价 + output×output价（`internal/storage/analytics.go` costForTokens）。
- 平台 cacheWrite 字段恒为 null → cache creation 按 input 价计。

## 历史数据回填（同日二次执行：逐帧回填）

第一轮回填用命中率估算；第二轮回填以 **OpenCode usage 页真实数据逐帧复原**（用户提供 server-fn 数据 648 条 usg 记录，含 `cacheReadTokens`/`cost`(units, 1e-8 USD)/`costMultiplier=2`/`timeCreated`/`provider`），并叠加 debug_capture 原始 usage：

- **平台精确层**：602 行（今天全量，含 09:59:57→11:24:25 与 12:50→13:02 UTC 段）按"时间 + output 相等"配对写入平台值（`cost_source='provider'`，`cost_usd=units/1e8`）；配对质量校验：总 token 误差 p99=0.0%、时间差中位数 5.4s。
- **capture 精确层**：383 行（11:50→14:05 UTC 段）用 capture `prompt_tokens^-^cached_tokens` 拆分并按请求时间戳对齐（`input_tokens=pt-cr`，cost 按 seed 价公式）。
- **剩余 estimated 179 行**（$0.58）：仅 11:10–11:15 部分与 **11:24:25→11:50:45 真缺口**——该窗口平台数据不存在（usage 页只返回最近 50 条，历史翻页未提供）且 capture 文件被轮换删除，无法精确；其余为新请求尾部（修复版本本已正确，仅标记）。
- 结果：**provider 985 行 = $2.32，estimated 179 行 = $0.58，总 $2.90**（今天全天）；窗口对账（09:59:21→11:24:25 UTC）provider 部分与平台 units 差 **0.4%**。
- **CompactGate**：`cached_input_tokens` 按时间轴对齐回填 1147 行（远端 start_time ↔ CompactGate time），与远端 `cache_read_tokens` 总量一致（≈258M，面板 263.7M 含最新请求）。
- 方法要点：平台记录 `input+cacheRead` = 代理全量，output 相等；capture 用 `upstream-request` 行时间戳（比响应完成时间更贴 start_time）；site 数据经 Edge CDP（9210 端口失败后启用 `chrome://inspect/#remote-debugging` 得 9222 WebSocket 端点）从 usage 页直接抓取，无需登录态注入。
- 教训：debug_capture 文件轮转（max_files 20 × 50MB）在流量峰值下几分钟即滚动，**回填/取证前应先拷贝 capture 目录**；usage 页服务端默认只渲染 50 条（早前窗口曾一次渲染 600 条——平台行为变化，翻页机制须用 CDP 实测）。
## Peak 定价验证与徽章现算（2026-08-27 增补）

- **平台峰值窗口实测边界**：逐条对比平台 `cost` 与 off-peak 公式（in×0.22 + cr×0.007 + out×0.66）——UTC 09:59:57 行 ×2.0，UTC 10:00:17 行 ×1.0。**窗口为 `[06:00, 10:00) UTC` 左闭右开**，与 `history.PeakMultiplier`（`internal/history/record.go:41`）和 `TestPeakMultiplier` 断言一致。
- **`costMultiplier: 2` 是 lite 计划固定标记，与 Peak 无关**：平台记录 `enrichment: {"plan":"lite","costMultiplier":2}` 恒为 2（`/tmp/platform_keep.json` 全量验证），**不计入 `cost` 字段**；`cost` 本身已是 peak 后最终值，对账直接 `units/1e8` 即可。
- **徽章前端现算**（commit `17f663f`）：`internal/gui/assets/app.js:1313` `effectivePeakMultiplier()` 按 `start_time + model` 前端判定 deepseek 工作日高峰，回填行不再依赖存储的 `peak_multiplier` 列（回填路径从不写该列，此前导致回填行集体丢徽章）。特效：存储值 >1 优先，避免平台记账时刻与 start_time 在边界（±秒级）判定分歧时徽章消失。
- **遗留时区 bug 修复**：9 行回填插入行 `start_time` 原存 UTC `+00:00`（回填脚本未转时区），按日统计错位 8 小时；已转 `+08:00`（备份 `backup-20260827-tzfix.db`，远端 `~/.local/share/routatic-proxy/`）。
- **2026-08-26/27 对账结论**：双方同时存在的行逐行成本差异为 0（1493+ 行精确到 1e-8）；8-26 远端 $4.9977 vs 平台 $4.9939（差 2 条平台未列出的记录 `$0.0015`），8-27 差异全为记账时差（平台 usage 页数据滞后约 5 分钟）。

## 重复计费溯源与对账去重（2026-08-27 增补二）

- **现象**：8-26 远端 $4.9977 vs 平台 $4.9939，差 $0.0038；8-27 差 $0.0022（记账时差）。
- **根因**：远端 3 条"重复行"——同一逻辑请求被 Claude Code 客户端超时重发（间隔 2–25 秒、`input+cache` 总和/output/cost 逐项相同、id 连号），代理将两次 HTTP 各记一行，平台合并为一次计费。`recordStreamSuccess`（`internal/handlers/messages.go:656`）每个 HTTP 请求只插入一行，插入逻辑本身不产生重复。
- **决策：代理插入层不做启发式去重**（token+时间窗判定有误删合法并发请求的风险）；正确姿势是**以平台为权威的对账去重**：逐行匹配（model+in+cr+out 全等或宽松 time+out+cost），平台有而远端多余的行直接 DELETE，总量对齐到 0.0001 USD。
- **8-27 覆盖回填**：57 行 matched 覆盖为平台精确值（`cost_source='provider'`），1 条 00:43:47 的在途请求保持 `estimated`（平台记账延迟 2–4 分钟，次日对账自然收敛）。
- **备份**：`backup-20260827-dedup.db`（删重）、`backup-20260827-backfill827.db`（回填）。

## CompactGate 溯源与方案取舍（2026-08-27 增补三）

- **CompactGate 侧无重发特征**：8-26 18:04+08 窗口内 out=560/214 各只有一条记录，全部 `success`、`client_disconnect_phase=none`、`upstream_status=200`；CG request_id（UUID）与 rproxy req-* 不同，**转发时未透传 X-Request-ID**。可知 Claude 客户端只发出一次请求且未断连；重复发生在 **CG→rproxy 网关转发层**（18:03:06 有 CG 记录 502 "socket hang up"，链路偶发断连触发网关层自动重试），rproxy 对同一逻辑请求（平台只计一次）记录两条。
- **插入层自动去重被否决**：60s 窗口 + (model, scenario, output, input+cr 总和) 指纹会误伤合法请求——`TestProviderCostReconciliationClassifiesWithoutGuessing` 中 1.2s 间隔的同指纹独立请求被吞（真实并行/快速连续请求同样可能）。**协议层无任何可读"重发特征"**（不透传 ID、无 retry header），因此"读特征再忽略"与"插入层剔除"均不可行。
- **最终决策：以平台为权威的对账去重**——平台按逻辑请求计费一次，远端多余记录由对账流程逐行匹配后删除（criteria：model+in+cr+out 全等，或宽松 time±120s+out+cost）。已在 8-26 执行 3 条（$0.0038）并验证远端 $4.9939 = 平台 $4.9939。

## 双日终局对账快照（2026-08-27 增补四）

- **修正上文**：8-26 的 3 条双写残留（`req-1787738698-19` / `req-1787738699-21` / `req-1787754050-471`，合计 $0.00375276）**当前仍在库中**——上文"已执行删除并验证 $4.9939=平台"是计划描述而非已达成状态（`backup-20260827-dedup827.db` 之后未再清理）。
- **8-26 终局**：远端 1504 行 $4.9977（provider 1498 + estimated 6）vs 平台 1501 行 $4.9939；唯一差异即上述 3 条残留（分别与 `usg_01M0YRFBHS` / `usg_01M0YRFFD` / `usg_01M0Z73TDS` 各重复一次）。尾部 6 条 estimated（23:54–23:56 北京）已被平台记账且估算值 = 平台值逐条相等——重建估算口径（peak ×2、cache 费率）验证正确。
- **8-27 终局（平台快照至 2026-08-27T07:10Z）**：远端 285 行 $3.2585 = provider 250 行 $2.2880（重建对齐段，与平台吻合）+ estimated 35 行 $0.9705；平台 282 行 $2.7904。35 条 estimated 中 33 条已出账、估算值 = 平台值；2 条大额（`req-1787814066-26` $0.235、`req-1787814207-28` $0.244）平台尚未出账，按 8-26 尾部先例稍后自然收敛。
- **结论**：两端费用口径一致；当前差异 = 一次性双写残留 $0.0038（清理即归零）+ 记账延迟尾部（自然收敛），无真实重复计费或遗漏。
