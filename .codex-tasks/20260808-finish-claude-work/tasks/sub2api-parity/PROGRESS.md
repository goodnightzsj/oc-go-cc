# Progress Log

## Context Recovery Block

- **Current milestone**: Rolling dashboard throughput semantics
- **Current status**: IN_PROGRESS
- **Last completed**: #28 - Implement and test rolling throughput
- **Current artifact**: `TODO.csv`
- **Key context**: Production `3bb6961` incorrectly labels selected-range average throughput as RPM/TPM; the fix must use the authoritative last 60 seconds without changing retained usage totals.
- **Known issues**: None. The unrelated remote `.ace-tool/` directory remains untouched.
- **Next action**: #29 - Validate, deploy, and smoke-test rolling throughput.

## Reopened Production Findings

- History and Analytics disagree because the prior release stored the complete provider snapshot separately instead of correcting the primary request history.
- The oversized indigo bar was the filtered History token trend compressed into one daily bucket; the History trend was removed at the user's request.
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
- History now exposes four filtered KPIs, full model/platform/scenario rows, metric switching, a structured Token tooltip, and unknown-detail semantics for fields the one-time correction cannot reconstruct; it does not render a daily trend.

## Production Retry

- Backup `20260810-175229-before-a9879e1.db` is mode `0600` and passed `PRAGMA integrity_check`.
- The first production dry run stopped before request writes because full config validation required a runtime API-key environment variable. The maintenance command now decodes only the storage block; a regression test confirms unresolved credential placeholders are irrelevant and never read.

## Production Validation

- Backup `20260810-190542-before-16cdc19.db` passed `PRAGMA integrity_check`, is mode `0600`, and preserves the pre-scenario-normalization database.
- Product commits `dede6d0`, `16cdc19`, and `3c0ded5` are pushed; production runs `3c0ded5` through the existing health/rollback deployment gate.
- Production has 1,390 requests, 463,249,833 Tokens, and `$3.03965577`; model/platform costs add up exactly and all 1,390 scenarios are `override`.
- The scenario correction updated 1,099 rows without inserts or removals; a second dry run reported zero changes.
- The existing Edge session on `https://opencode.9962510.xyz/` verified desktop and 400x739 responsive layouts, no document overflow, Chinese modal/tooltips, themed controls, no source/recent/History-trend sections, and tooltip dismissal on tab changes.
- Real `Cmd+R` and `Cmd+Shift+R` keystrokes cleared an in-memory probe and reported Navigation Timing type `reload` on UI build `6ca66216bc0f`.

## Verified Local Reference

- Running CompactGate `#analytics` contains six metrics, request and Token trends, platform/model distributions, and a model-path table.
- Running CompactGate `#usage` contains seven metrics, a four-series Token/cache-rate trend, a period table, a model table, and endpoint distribution.
- Running CompactGate `#logs` contains themed filters, a compact request table, a structured Token tooltip, and expandable request details.
- This project can truthfully port all of those except endpoint, first-Token, and model-path fields, which are not present in its request contract.

## Verified Port

- Dashboard now follows the real CompactGate analytics structure: six KPIs, request and Token line trends, and model/platform horizontal distributions.
- Usage now follows the real CompactGate usage structure: custom date range, hour/day granularity, Token/cache-rate trend, period details, model details, and platform distribution.
- Request History now keeps model/platform visible and exposes a themed structured Token tooltip; source provenance and recent-usage sections remain absent.
- History status cells now identify streaming and non-streaming requests while preserving unknown imported details.
- Full repository tests, vet, formatting, JavaScript syntax, diff checks, and the CGO-disabled production build pass locally.
- Production `3bb6961` is deployed; public health and API checks pass, and the existing Edge public-domain session verified the new Dashboard and History behavior.
- The guessed donut and single-bucket stacked-bar implementations were deleted. Full Go tests, build, JavaScript syntax, and diff checks pass.
