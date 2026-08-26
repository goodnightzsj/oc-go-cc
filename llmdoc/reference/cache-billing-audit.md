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
| 远端代理 | `requests` 表 | 今天 09:59:21 UTC 起 607 行 | input 存"全量"（修复前）/ "纯输入"（修复后），缓存分列 |
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

## 历史数据回填（同日执行）

远端 `requests` 表修复前行的数据已按"与 OpenCode 账单一致"回填：

- **精确层**：7 行（重叠窗口）直接写平台账单值（含平台 cost，`cost_source='provider'`）；27 行（部署前最后 4.5 分钟）用 capture 原始 `cached_tokens` 拆分并重算（`estimated`）。
- **估算层**：其余修复前行无原始缓存数据，按 capture 窗口命中率 99.64% 拆分 input/cache_read 并重算（`estimated`）——早期会话真实命中率更低（重叠窗口实测 80.2%），该层为近似值。
- 回填前总成本 $18.00（全量×0.14）→ 回填后 $2.01（与按窗口比例推算的平台全天账单 $1.5-2 吻合）。
- 验证关系：平台账单 input+cacheRead = 代理原始全量（重叠窗口 7/7 按此指纹配对成功）；DB 已备份 `data.db.backup-2608`。