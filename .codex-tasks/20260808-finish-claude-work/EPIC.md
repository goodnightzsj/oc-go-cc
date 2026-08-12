# Finish Claude Session Work

## Goal

- Complete the verified unfinished requirements across the full continuation chain, reconcile production request costs with a fresh authenticated OpenCode usage capture, finish the sub2api-informed dashboard and History experience, then validate and deploy them.

## Non-Goals

- Keep provider provenance or reconciliation details visible in the product UI.
- Port sub2api backend, billing, tenancy, or unrelated SaaS features.
- Replace the existing dependency-free dashboard stack.
- Persist browser credentials, request content, or provider authentication material.

## Constraints

- Preserve existing public behavior unless a verified bug requires a change.
- Do not expose credentials in code, logs, task artifacts, or commits.
- Reuse the existing Go, SQLite, embedded HTML/CSS/JavaScript, SVG, and systemd deployment paths.
- Dispatch at most one subagent at a time.

## Risk Assessment

- Configuration overlay changes can silently disable persistence and analytics routes.
- Secret redaction and config persistence changes affect a trust boundary.
- Analytics token/cost definitions must stay consistent across summary, charts, and history.
- Remote deployment changes live service state and must retain rollback and health checks.
- OpenCode usage reconciliation depends on an authenticated local Edge session.
- Production data correction requires a backup, deterministic dry-run matching, and an explicit mismatch report before writes.

## Child Deliverables

- Finalize the storage overlay regression fix.
- Verify and fix security and streaming P0 findings.
- Fix verified analytics/dashboard correctness and accessibility issues.
- Port only the useful sub2api data-presentation patterns.
- Reconcile estimated costs with real OpenCode usage data.
- Protect SQLite catalog credentials at rest or remove the unnecessary plaintext persistence path.
- Recapture current OpenCode subscription and per-request usage, then reconcile provider-reported costs with trustworthy proxy rows.
- Finish the sub2api-informed Analytics and History design with responsive, keyboard-accessible drill-down workflows.
- Complete the reopened Sub2API parity request with an OpenCode-authoritative usage baseline, themed controls, Chinese request details, and browser-native refresh behavior.
- Correct primary request history once from the complete provider snapshot and finish the dense source-neutral chart experience.
- Run full validation, commit, push, deploy, and smoke-test production.

## Dependency Notes

- Security and dashboard work start after the current storage regression is locked down.
- Frontend porting follows dashboard correctness and design-context setup.
- SQLite secret hardening precedes provider-data import work.
- Provider reconciliation defines the cost provenance shown by the final dashboard.
- Dashboard and History implementation precede the final release validation.
- Deployment follows every local deliverable and validation gate.

## Done-When

- [x] Every row in `SUBTASKS.csv` is `DONE`.
- [x] Full Go validation and focused frontend checks pass.
- [x] Production service, health endpoint, GUI, analytics JSON, History totals, and Edge interactions pass.
