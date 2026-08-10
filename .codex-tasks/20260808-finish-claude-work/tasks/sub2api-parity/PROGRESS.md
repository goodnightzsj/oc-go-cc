# Progress Log

## Context Recovery Block

- **Current milestone**: Deploy and validate the verified CompactGate port
- **Current status**: IN_PROGRESS
- **Last completed**: #12 - Port verified dashboard, usage, and log patterns
- **Current artifact**: `TODO.csv`
- **Key context**: Production History and Analytics now share 1,390 trusted request rows totaling `$3.03965577`; provider labels are normalized to `opencode-go`.
- **Known issues**: None in local checks; production still runs the previous release until task #13 deploys this commit.
- **Next action**: #13 - deploy and verify the public-domain desktop/narrow layouts, custom tooltips, dialog, and browser refresh in the existing Edge session.

## Reopened Production Findings

- History and Analytics disagree because the prior release stored the complete provider snapshot separately instead of correcting the primary request history.
- The oversized indigo bar is the filtered History token trend compressed into one daily bucket; it has insufficient context and will be replaced by a labelled, multi-metric time-series treatment.
- Provider provenance is an implementation detail and must not be shown in Analytics.
- The final release must be tested remotely in Edge through `https://opencode.9962510.xyz/` only.

## Reference Audit

- Sub2API commit `635ad81` confirms the useful structure is KPI rows, multi-series Token trend, metric-switchable model/platform distributions, compact breakdown tables, filters, and request rows.
- SaaS-only user, API-key, IP, endpoint-mapping, and three-price fields are intentionally excluded because this project has no trustworthy source for them.
- Production read-only SQLite evidence: 1,062 request rows total `$24.56317345` (772 estimated rows total `$24.12131848`); 1,390 provider rows total `$3.03965577`; 290 request rows already have exact provider costs totaling `$0.44185497`.

## Implementation

- `costs sync-requests` reads the persisted sanitized snapshot, reports exact projected totals without writes, and applies one transactional correction with stable request IDs.
- Corrected usage is marked internally as trusted so the configured legacy baseline cannot hide repaired rows; future requests continue through the normal request insert path and the UI reads only the primary request aggregates.
- Dashboard and Analytics now share request-backed KPIs, four-series Token trend, model/platform distributions, Token/cost/request metric switching, and themed pointer/keyboard/touch tooltips.
- History now exposes four filtered KPIs, full model/platform/scenario rows, metric switching, a labelled trend, and unknown-detail semantics for fields the one-time correction cannot reconstruct.

## Production Retry

- Backup `20260810-175229-before-a9879e1.db` is mode `0600` and passed `PRAGMA integrity_check`.
- The first production dry run stopped before request writes because full config validation required a runtime API-key environment variable. The maintenance command now decodes only the storage block; a regression test confirms unresolved credential placeholders are irrelevant and never read.

## Production Validation

- Backup `20260810-165925-before-cc7b0d9.db` passed `PRAGMA integrity_check` and is mode `0600`.
- Product commits `cc7b0d9`, `60bc714`, and `d0d8fa5` are pushed; production runs `d0d8fa5` through the existing health/rollback deployment gate.
- Edge on `https://opencode.9962510.xyz/` verified the Chinese request dialog, themed selects/date range, model/provider/plan charts, token trend, recent usage, History summaries, and zero desktop/mobile document overflow.
- Provider and plan labels resolve to `inf-go.oa-compat` and `lite`; refresh events for `Meta+R`, `F5`, and `Meta+Shift+R` are not prevented.

## Verified Local Reference

- Running CompactGate `#analytics` contains six metrics, request and Token trends, platform/model distributions, and a model-path table.
- Running CompactGate `#usage` contains seven metrics, a four-series Token/cache-rate trend, a period table, a model table, and endpoint distribution.
- Running CompactGate `#logs` contains themed filters, a compact request table, a structured Token tooltip, and expandable request details.
- This project can truthfully port all of those except endpoint, first-Token, and model-path fields, which are not present in its request contract.

## Verified Port

- Dashboard now follows the real CompactGate analytics structure: six KPIs, request and Token line trends, and model/platform horizontal distributions.
- Usage now follows the real CompactGate usage structure: custom date range, hour/day granularity, Token/cache-rate trend, period details, model details, and platform distribution.
- Request History now keeps model/platform visible and exposes a themed structured Token tooltip; source provenance and recent-usage sections remain absent.
- The guessed donut and single-bucket stacked-bar implementations were deleted. Full Go tests, build, JavaScript syntax, and diff checks pass.
