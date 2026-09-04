# Cost Mismatch Troubleshooting Guide

When the remote dashboard's cost statistics don't match OpenCode's official usage page, investigate these factors in order.

## 1. Data Integrity Issues

### Corrupted Records (Zero-Token Entries)

**Symptom:** Remote shows higher cost than OpenCode with extra records that have `input_tokens = 0`, `output_tokens = 0`, `cache_read_tokens = 0` but non-zero `cost_usd`.

**Root cause:** Malformed import from sync scripts. These records contribute to cost aggregation but represent no actual traffic.

**This was the primary cause in the 2026-09-04 investigation:** 14 corrupted records with $0.30 phantom cost were found and deleted, immediately aligning remote total with OpenCode.

**Detection:**
```sql
SELECT 
  COUNT(*) as corrupted_count,
  SUM(cost_usd) as phantom_cost
FROM requests 
WHERE (input_tokens = 0 AND output_tokens = 0 AND cache_read_tokens = 0)
  AND cost_usd > 0
  AND datetime(start_time, 'localtime') >= date('now', 'localtime');
```

**Fix:** Delete corrupted records after verifying they're not legitimate zero-token requests (prompt caching hits, embeddings):
```sql
DELETE FROM requests 
WHERE id IN (
  SELECT id FROM requests 
  WHERE (input_tokens = 0 AND output_tokens = 0 AND cache_read_tokens = 0)
    AND cost_usd > 0
    AND datetime(start_time, 'localtime') >= date('now', 'localtime')
);
```

**Prevention:** 
- Always validate sync data before import
- Ensure `oc_sync.py` parses all token fields (input, output, cache_read)
- **Split cache tokens when importing**: OpenCode reports `cacheReadTokens` as part of input in their ledger, but the proxy DB stores them separately. When importing, extract cache tokens from the input count to avoid double-counting.

### Duplicate Records

**Symptom:** Same `id` appears multiple times in remote DB, inflating counts and costs.

**Detection:**
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

**Fix:** Keep the first occurrence, delete duplicates:
```sql
DELETE FROM requests 
WHERE rowid NOT IN (
  SELECT MIN(rowid) 
  FROM requests 
  GROUP BY REPLACE(id, 'oc-', '')
);
```

## 2. Time Window Alignment

### Timezone Differences

**Issue:** Remote server uses local time (CST) for daily aggregation, but baseline sync may use UTC cutoffs.

**Verification:**
```bash
# Remote server timezone
ssh root@<server> 'date +%Z'

# Check if daily window boundaries align
sqlite3 data.db "
SELECT 
  datetime(MIN(start_time), 'localtime') as first,
  datetime(MAX(start_time), 'localtime') as last
FROM requests 
WHERE datetime(start_time, 'localtime') >= date('now', 'localtime');
"
```

**Fix:** Ensure sync scripts use the same timezone as the remote DB aggregation (`date('now', 'localtime')` for CST).

### Subscription Cycle vs Calendar Month

**Issue:** OpenCode plan windows use 31-day subscription cycles anchored to `resets_at`, not calendar months.

The monthly usage window is `[resets_at - 31 days, resets_at)`, not `[first day of month, last day of month]`.

**Example:**
- Plan resets: `2026-09-06T06:44:54Z`
- Monthly window: `2026-08-06 06:44:54` → `2026-09-06 06:44:54` (31 days)
- Calendar September: `2026-09-01 00:00:00` → `2026-09-30 23:59:59` (30 days)

**Verification:** Compare `quota.go` window logic with OpenCode's actual reset timestamp.

## 3. Proxy Bypass

**Issue:** Some requests went directly to OpenCode (bypassed the proxy), so they appear on OpenCode's page but not in the remote DB.

**Detection:**
```bash
# Find gaps in remote timeline
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

If there's a multi-hour gap with 0 records but OpenCode shows traffic during that period, requests bypassed the proxy.

**Causes:**
- User switched API key/endpoint in their client
- Direct `curl` tests to OpenCode without going through `127.0.0.1:3456`
- Claude Code configured with `ANTHROPIC_BASE_URL` pointing directly to OpenCode

**Fix:** 
1. Verify Claude Code environment:
   ```bash
   echo $ANTHROPIC_BASE_URL  # Should be http://127.0.0.1:3456
   echo $ANTHROPIC_AUTH_TOKEN  # Can be any non-empty value
   ```
2. Check proxy is running: `ps aux | grep routatic-proxy`

## 4. Cost Calculation Mismatches

### Pool Multiplier Confusion

**Issue:** OpenCode Go uses per-model pool multipliers (DeepSeek ×2 = $30 allowance consumes $60 from shared pool). If cost aggregation mixes price currency with pool equivalents, totals won't match.

**Correct semantics:**
- **Row spend:** Official price currency (`used_usd = scale × windowSum / poolMultiplier`), directly comparable to docs allowance
- **Total row:** Pool-equivalent (`poolUsed = Σ(row.used × 60/row.allowance)`), reconciles with OpenCode's monthly percent

**Verification:** See `internal/gui/quota.go` line 150-180 and commit `79c8811`.

### Baseline Drift

**Issue:** Local baseline TSV (`~/.routatic-sync/records.tsv`) gets out of sync with remote DB if:
- Manual imports bypass the baseline update
- TSV file is deleted/truncated
- Sync script crashes mid-run

**Detection:**
```bash
# Count baseline records for today
awk -F'\t' '$2 >= "2026-09-04T00:00:00"' ~/.routatic-sync/records.tsv | wc -l

# Compare with remote
ssh root@<server> "sqlite3 data.db 'SELECT COUNT(*) FROM requests WHERE ...'"
```

**Fix:** Re-sync missing records from remote to baseline:
```python
# Fetch remote records not in baseline, append to TSV
remote_ids = set(fetch_from_remote())
baseline_ids = set(parse_baseline())
missing = remote_ids - baseline_ids
append_to_baseline(missing)
```

## Diagnostic Checklist

Run these checks in order:

```bash
# 1. Check for corrupted records (primary cause)
sqlite3 data.db "
SELECT COUNT(*), SUM(cost_usd) 
FROM requests 
WHERE input_tokens = 0 AND output_tokens = 0 AND cache_read_tokens = 0 AND cost_usd > 0;
"

# 2. Check for duplicates
sqlite3 data.db "
SELECT COUNT(*) - COUNT(DISTINCT REPLACE(id, 'oc-', '')) as duplicate_count
FROM requests WHERE id LIKE 'oc-%';
"

# 3. Verify today's count and cost
sqlite3 data.db "
SELECT COUNT(*), ROUND(SUM(cost_usd), 6)
FROM requests 
WHERE datetime(start_time, 'localtime') >= date('now', 'localtime');
"

# 4. Compare with OpenCode official page
# Log into opencode.ai/workspace/<workspace_id>/usage and check the daily/monthly totals

# 5. Check proxy is capturing traffic
echo \$ANTHROPIC_BASE_URL  # Should be http://127.0.0.1:3456
ps aux | grep routatic-proxy | grep -v grep
```

## Common Patterns

| Symptom | Likely Cause | Fix |
|---------|--------------|-----|
| Remote > OpenCode by small constant $ | Corrupted zero-token records | Delete phantom records (see section 1) |
| Remote > OpenCode by exact duplicate count | Duplicate IDs | Deduplicate by ID |
| Remote < OpenCode, with timeline gaps | Proxy bypass | Verify `ANTHROPIC_BASE_URL` points to proxy |
| Monthly total wrong but daily correct | Pool multiplier mixup | Fix `quota.go` aggregation logic |
| Daily window off by hours | Timezone mismatch | Use `localtime` consistently |

## Related Files

- `internal/gui/quota.go` — Monthly model usage aggregation logic (pool multiplier handling)
- `internal/storage/requests.go` — Cost calculation (`EstCostUSD`)
- `/tmp/oc_sync.py` — Incremental sync from OpenCode usage.list to local TSV + remote DB
- `docs/architecture.md` — Overall request flow and cost tracking


# 4. Compare with OpenCode official page
# Log into opencode.ai/workspace/<workspace_id>/usage and check the daily/monthly totals

# 5. Check proxy is capturing traffic
echo $ANTHROPIC_BASE_URL  # Should be http://127.0.0.1:3456
ps aux | grep routatic-proxy | grep -v grep
```

## Common Patterns

| Symptom | Likely Cause | Fix |
|---------|--------------|-----|
| Remote > OpenCode by small constant $ | Corrupted zero-token records | Delete phantom records (see section 1) |
| Remote > OpenCode by exact duplicate count | Duplicate IDs | Deduplicate by ID |
| Remote < OpenCode, with timeline gaps | Proxy bypass | Verify `ANTHROPIC_BASE_URL` points to proxy |
| Monthly total wrong but daily correct | Pool multiplier mixup | Fix `quota.go` aggregation logic |
| Daily window off by hours | Timezone mismatch | Use `localtime` consistently |

## Related Files

- `internal/gui/quota.go` — Monthly model usage aggregation logic (pool multiplier handling)
- `internal/storage/requests.go` — Cost calculation (`EstCostUSD`)
- `/tmp/oc_sync.py` — Incremental sync from OpenCode usage.list to local TSV + remote DB
- `docs/architecture.md` — Overall request flow and cost tracking

