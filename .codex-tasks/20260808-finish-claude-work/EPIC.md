# Finish Claude Session Work

## Goal

- Complete the verified unfinished requirements from Claude session `78af710c-35ca-4fc2-8aa0-a30f6273dc16`, then validate and deploy them.

## Non-Goals

- Reconstruct analytics rows recorded before `analytics_baseline`.
- Port sub2api backend, billing, tenancy, or unrelated SaaS features.
- Replace the existing dependency-free dashboard stack.

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

## Child Deliverables

- Finalize the storage overlay regression fix.
- Verify and fix security and streaming P0 findings.
- Fix verified analytics/dashboard correctness and accessibility issues.
- Port only the useful sub2api data-presentation patterns.
- Reconcile estimated costs with real OpenCode usage data.
- Run full validation, commit, push, deploy, and smoke-test production.

## Dependency Notes

- Security and dashboard work start after the current storage regression is locked down.
- Frontend porting follows dashboard correctness and design-context setup.
- Deployment follows every local deliverable and validation gate.

## Done-When

- [ ] Every row in `SUBTASKS.csv` is `DONE`.
- [ ] Full Go validation and focused frontend checks pass.
- [ ] Production service, health endpoint, GUI, and analytics JSON smoke tests pass.
