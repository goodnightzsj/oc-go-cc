# Release Validation and Smoke Test

## Task Shape

- **Shape**: single-full

## Goals

- Run the complete local validation for the already-scoped changes.
- Commit and push only the verified project/task artifacts when repository state is cleanly attributable.
- Deploy through `scripts/prod-deploy.sh` on the configured production checkout, preserving its rollback and health checks.
- Verify health, GUI, and analytics JSON responses after deployment.

## Non-Goals

- Do not repeat OpenCode browser capture or alter provider/account data.
- Do not rewrite history, reset unrelated worktree changes, or migrate the database.
- Do not expose credentials or request content.

## Constraints

- Use the existing Go, systemd, and `scripts/prod-deploy.sh` paths.
- Remote commands must use the configured SSH target and non-interactive checks.
- Stop before any remote write if host authentication or target identity cannot be verified.

## Risk Assessment

- Remote deployment changes live service state; the deploy script must be the only restart/rollback path.
- A failed remote health check must leave the previous release selected.
- Existing dirty files may include user work; review the complete diff before commit.

## Deliverables

- Local validation evidence.
- Release commit and remote deployment result, or a recorded external blocker.
- Post-deploy health, GUI, and analytics endpoint smoke evidence.

## Done-When

- [ ] Local full validation passes.
- [ ] Verified changes are committed and pushed.
- [ ] Production deploy completes with health check, or the external blocker is explicitly recorded.
- [ ] Health, GUI, and analytics JSON smoke checks pass.

## Final Validation Command

```bash
go test ./... && go vet ./... && test -z "$(gofmt -l cmd internal pkg)"
```
