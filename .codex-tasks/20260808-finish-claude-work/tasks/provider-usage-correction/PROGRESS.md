# Progress Log

## Context Recovery Block

- **Current milestone**: Complete
- **Current status**: DONE
- **Last completed**: #4 - Prepare production backup and dry run
- **Current artifact**: `TODO.csv`
- **Key context**: The old capture proved aggregate agreement for 290 rows but did not create an exact-row provider-cost import or retain subscription-cycle evidence.
- **Known issues**: None for the trusted post-baseline rows; the 772 pre-baseline rows intentionally retain estimated provenance.
- **Next action**: None.

## 2026-08-10 - Fresh authenticated Edge capture

- Source: existing authenticated Edge tab at the requested OpenCode workspace Usage route.
- Period shown by the page: `August 2026`; per-row plan: `lite`.
- Coverage: 28 pages, 1,390 rows, from `2026-08-06T06:56:37Z` through `2026-08-06T13:37:43Z`.
- Provider totals: 11,402,970 input, 565,578 output, 70,578 reasoning, 451,281,285 cache-read tokens, and `$3.03965577`.
- Models: 1,389 `deepseek-v4-flash` rows (`$3.01474102`) and one `kimi-k2.6` row (`$0.02491475`).
- Sanitized matching rows are temporarily stored at `/tmp/opencode-usage-sanitized.json` with mode `0600`; no cookie, authorization material, request content, email, or browser response body is retained.

## 2026-08-10 - Production connectivity

- A verbose BatchMode identity check established TCP to port 22, then the production host closed during SSH key exchange before authentication.
- No remote backup, deploy, database read, or write has occurred in this resumed task.

## 2026-08-10 - Deterministic reconciliation implementation

- Exact identity uses UTC second, normalized model basename, input/output tokens, cache reads, and total cache writes.
- Duplicate provider or proxy identities are classified as ambiguous; token disagreements at the same timestamp/model are classified as conflicts; missing provider-account rows remain non-writing observations.
- `cost_source` distinguishes `estimated` from `provider` across SQLite, History, CSV, detail views, filters, and Analytics coverage.
- `routatic-proxy costs reconcile --input FILE` prints the dry-run report; `--apply` is transactional, idempotent, and refuses ambiguous batches.
- Storage, GUI, CLI, full Go tests, vet, formatting, JavaScript syntax, and diff checks pass locally; the final production matching result is recorded below.

## 2026-08-10 - Production reconciliation complete

- Backup `/root/oc-go-cc/.tmp/prod/backups/20260810-155140-before-a55e101.db` is mode `0600` and passed `PRAGMA integrity_check`.
- Production evidence established that OpenCode timestamps the provider completion second, while proxy response handling finishes in that second or the next; full token fingerprints are unique on both sides after the configured baseline.
- Final dry run: 290 exact, zero ambiguous, zero missing, zero conflicting, `$0.44185497`.
- Transactional apply updated 290 rows; the second dry run reported `would_update=0`.
- Production Analytics reports 290 provider-cost rows and zero estimated rows inside the trusted baseline window.
