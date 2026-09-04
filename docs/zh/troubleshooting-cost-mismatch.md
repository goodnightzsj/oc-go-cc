# Cost 对账问题排查指南

当远端 Dashboard 的费用统计与 OpenCode 官方用量页面不一致时，按以下顺序排查。

## 1. 数据完整性问题

### 脏数据（零 Token 记录）

**症状**：远端显示的费用高于 OpenCode，且存在 `input_tokens = 0`、`output_tokens = 0`、`cache_read_tokens = 0` 但 `cost_usd` 非零的记录。

**根因**：同步脚本导入的畸形记录。这些记录参与费用聚合但不代表真实流量。

**这是 2026-09-04 排查中的主要原因**：发现并删除了 14 条脏数据，虚增 $0.30 费用，删除后远端总额立即与 OpenCode 对齐。

**检测**：
```sql
SELECT 
  COUNT(*) as corrupted_count,
  SUM(cost_usd) as phantom_cost
FROM requests 
WHERE (input_tokens = 0 AND output_tokens = 0 AND cache_read_tokens = 0)
  AND cost_usd > 0
  AND datetime(start_time, 'localtime') >= date('now', 'localtime');
```

**修复**：验证这些不是合法的零 token 请求（prompt caching 命中、embeddings）后删除：
```sql
DELETE FROM requests 
WHERE id IN (
  SELECT id FROM requests 
  WHERE (input_tokens = 0 AND output_tokens = 0 AND cache_read_tokens = 0)
    AND cost_usd > 0
    AND datetime(start_time, 'localtime') >= date('now', 'localtime')
);
```

**预防**：
- 导入前验证同步数据
- 确保 `oc_sync.py` 解析所有 token 字段（input、output、cache_read）
- **导入时拆分缓存 token**：OpenCode 账本中 `cacheReadTokens` 计入 input，但代理 DB 单独存储。导入时需从 input 中提取缓存 token，避免重复计数

### 重复记录

**症状**：同一个 `id` 在远端 DB 中出现多次，虚增计数和费用。

**检测**：
```sql
SELECT 
  REPLACE(id, 'oc-', '') as clean_id,
  COUNT(*) as occurrences,
  SUM(cost_usd) as total_cost
FROM requests 
WHERE id LIKE 'oc-%'
GROUP BY clean_id
HAVING COUNT(*) > 1;
```

**修复**：保留首次出现，删除重复：
```sql
DELETE FROM requests 
WHERE rowid NOT IN (
  SELECT MIN(rowid) 
  FROM requests 
  GROUP BY REPLACE(id, 'oc-', '')
);
```

## 2. 时间窗口对齐

### 时区差异

**问题**：远端服务器用本地时间（CST）做日聚合，但 baseline 同步可能用 UTC 截断。

**验证**：
```bash
# 远端服务器时区
ssh root@<server> 'date +%Z'

# 检查日窗口边界是否对齐
sqlite3 data.db "
SELECT 
  datetime(MIN(start_time), 'localtime') as first,
  datetime(MAX(start_time), 'localtime') as last
FROM requests 
WHERE datetime(start_time, 'localtime') >= date('now', 'localtime');
"
```

**修复**：确保同步脚本与远端 DB 聚合使用相同时区（CST 用 `date('now', 'localtime')`）。

### 订阅周期 vs 日历月

**问题**：OpenCode 套餐窗口用 31 天订阅周期（锚定 `resets_at`），不是日历月。

月度用量窗口是 `[resets_at - 31 天, resets_at)`，不是 `[月初, 月末]`。

**示例**：
- 套餐重置：`2026-09-06T06:44:54Z`
- 月度窗口：`2026-08-06 06:44:54` → `2026-09-06 06:44:54`（31 天）
- 日历九月：`2026-09-01 00:00:00` → `2026-09-30 23:59:59`（30 天）

**验证**：对比 `quota.go` 窗口逻辑与 OpenCode 实际 reset 时间戳。

## 3. 代理旁路

**问题**：部分请求直接发往 OpenCode（绕过代理），所以出现在 OpenCode 页面但不在远端 DB 中。

**检测**：
```bash
# 查找远端时间线空白
sqlite3 data.db "
SELECT 
  datetime(start_time, 'localtime') as ts,
  COUNT(*) as count
FROM requests 
WHERE datetime(start_time, 'localtime') >= date('now', 'localtime')
GROUP BY strftime('%Y-%m-%d %H:00', start_time, 'localtime')
ORDER BY ts;
"
```

如果存在多小时空白（0 条记录）但 OpenCode 在该时段有流量，则请求绕过了代理。

**原因**：
- 用户在客户端切换了 API key/endpoint
- 直接用 `curl` 测试 OpenCode 而不经过 `127.0.0.1:3456`
- Claude Code 的 `ANTHROPIC_BASE_URL` 直接指向 OpenCode

**修复**：
1. 验证 Claude Code 环境：
   ```bash
   echo $ANTHROPIC_BASE_URL  # 应为 http://127.0.0.1:3456
   echo $ANTHROPIC_AUTH_TOKEN  # 可以是任意非空值
   ```
2. 检查代理是否运行：`ps aux | grep routatic-proxy`

## 4. 费用计算不匹配

### 池倍率混淆

**问题**：OpenCode Go 使用模型池倍率（DeepSeek ×2 = $30 配额消耗 $60 共享池）。如果费用聚合混合了价格币种和池等效，总额将不匹配。

**正确语义**：
- **行金额**：官方价格币种（`used_usd = scale × windowSum / poolMultiplier`），直接与文档配额对比
- **总计行**：池等效（`poolUsed = Σ(row.used × 60/row.allowance)`），与 OpenCode 月度百分比对齐

**验证**：见 `internal/gui/quota.go` 第 150-180 行和提交 `79c8811`。

### Baseline 漂移

**问题**：本地 baseline TSV（`~/.routatic-sync/records.tsv`）与远端 DB 失去同步，如果：
- 手动导入绕过了 baseline 更新
- TSV 文件被删除/截断
- 同步脚本中途崩溃

**检测**：
```bash
# 统计 baseline 今日记录数
awk -F'\t' '$2 >= "2026-09-04T00:00:00"' ~/.routatic-sync/records.tsv | wc -l

# 与远端对比
ssh root@<server> "sqlite3 data.db 'SELECT COUNT(*) FROM requests WHERE ...'"
```

**修复**：从远端重新同步缺失记录到 baseline：
```python
# 抓取远端有但 baseline 没有的记录，追加到 TSV
remote_ids = set(fetch_from_remote())
baseline_ids = set(parse_baseline())
missing = remote_ids - baseline_ids
append_to_baseline(missing)
```

## 诊断检查清单

按顺序运行这些检查：

```bash
# 1. 检查脏数据（主要原因）
sqlite3 data.db "
SELECT COUNT(*), SUM(cost_usd) 
FROM requests 
WHERE input_tokens = 0 AND output_tokens = 0 AND cache_read_tokens = 0 AND cost_usd > 0;
"

# 2. 检查重复
sqlite3 data.db "
SELECT COUNT(*) - COUNT(DISTINCT REPLACE(id, 'oc-', '')) as duplicate_count
FROM requests WHERE id LIKE 'oc-%';
"

# 3. 验证今日计数和费用
sqlite3 data.db "
SELECT COUNT(*), ROUND(SUM(cost_usd), 6)
FROM requests 
WHERE datetime(start_time, 'localtime') >= date('now', 'localtime');
"

# 4. 与 OpenCode 官方页面对比
# 登录 opencode.ai/workspace/<workspace_id>/usage 检查日/月总额

# 5. 检查代理是否在捕获流量
echo $ANTHROPIC_BASE_URL  # 应为 http://127.0.0.1:3456
ps aux | grep routatic-proxy | grep -v grep
```

## 常见模式

| 症状 | 可能原因 | 修复 |
|------|---------|-----|
| 远端 > OpenCode 小额恒定差 | 脏数据（零 token 记录） | 删除幽灵记录（见第 1 节） |
| 远端 > OpenCode 精确重复倍数 | 重复 ID | 按 ID 去重 |
| 远端 < OpenCode，时间线有空白 | 代理旁路 | 验证 `ANTHROPIC_BASE_URL` 指向代理 |
| 月度总计错误但日总计正确 | 池倍率混淆 | 修复 `quota.go` 聚合逻辑 |
| 日窗口偏移数小时 | 时区不匹配 | 统一使用 `localtime` |

## 相关文件

- `internal/gui/quota.go` — 月度模型用量聚合逻辑（池倍率处理）
- `internal/storage/requests.go` — 费用计算（`EstCostUSD`）
- `/tmp/oc_sync.py` — 从 OpenCode usage.list 增量同步到本地 TSV + 远端 DB
- `docs/architecture.md` — 整体请求流程和费用追踪

