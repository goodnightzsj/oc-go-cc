# Progress Log

## Context Recovery Block

- **Current milestone**: Correct request truth and finish the Sub2API-informed data experience
- **Current status**: IN_PROGRESS
- **Last completed**: #9 - Complete local release validation
- **Current artifact**: `TODO.csv`
- **Key context**: Production Analytics has 1,390 provider rows totaling `$3.03965577`, but History still has 1,062 mixed-quality local rows totaling `$24.56317345`; 772 pre-baseline costs are estimates.
- **Known issues**: The data-source and recent-usage sections conflict with the clarified product contract; the History mini trend degenerates into an unlabeled single bar; charts and tooltips remain incomplete.
- **Next action**: #10 - back up, deploy, dry-run/apply the production correction, and validate the public domain with Edge.

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

## Production Validation

- Backup `20260810-165925-before-cc7b0d9.db` passed `PRAGMA integrity_check` and is mode `0600`.
- Product commits `cc7b0d9`, `60bc714`, and `d0d8fa5` are pushed; production runs `d0d8fa5` through the existing health/rollback deployment gate.
- Edge on `https://opencode.9962510.xyz/` verified the Chinese request dialog, themed selects/date range, model/provider/plan charts, token trend, recent usage, History summaries, and zero desktop/mobile document overflow.
- Provider and plan labels resolve to `inf-go.oa-compat` and `lite`; refresh events for `Meta+R`, `F5`, and `Meta+Shift+R` are not prevented.
