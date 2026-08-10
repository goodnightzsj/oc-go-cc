# Progress Log

## Session Start

- **Date**: 2026-08-08 03:26 +08:00
- **Task name**: `release`
- **Task dir**: `.codex-tasks/20260808-finish-claude-work/tasks/release/`
- **Spec**: See `SPEC.md`
- **Plan**: See `TODO.csv` (4 milestones)
- **Environment**: Go / embedded GUI / systemd / shell smoke checks

## Context Recovery Block

- **Current milestone**: Complete
- **Current status**: DONE
- **Last completed**: #4 - Run production smoke checks
- **Current artifact**: `TODO.csv`
- **Key context**: The user reopened exact provider-cost correction, SQLite secret persistence, and the sub2api-informed dashboard/History work. Earlier local validation and commit evidence is now stale for release purposes.
- **Known issues**: None for this release; remote `.ace-tool/` remains an unrelated untracked directory and was not modified.
- **Next action**: None.

## Final release

- Product commits: `a55e101`, `d4e5449`, `aecbe1c`, all pushed to `origin/main`.
- Production release: `/root/oc-go-cc/.tmp/prod/releases/20260810160751-23eade682ab0`.
- Health reports `aecbe1c95412+dirty`; the dirty marker comes from the pre-existing remote `.ace-tool/` and does not change the source fingerprint.
- SQLite database, WAL, and SHM are mode `0600`; legacy catalog API keys are cleared.
- Production backup, exact-cost reconciliation, GUI/API smoke checks, and Edge desktop/mobile validation passed.

## Milestone 1: Run full local validation

- **Status**: DONE
- **Completed**: 2026-08-10 14:28 +08:00
- **Validation**: `go test ./... && go vet ./... && test -z "$(gofmt -l cmd internal pkg)" && git diff --check` passed after formatting `internal/storage/analytics.go`.

## Milestone 2: Review and create release commit

- **Status**: DONE
- **Completed**: 2026-08-10 14:36 +08:00
- **Validation**: Staged diff and secret checks passed; commit `79604ab` was pushed to `origin/main`.

## Milestone 3: Deploy through the existing script

- **Status**: FAILED
- **Attempted**: 2026-08-10 14:39 +08:00
- **Validation**: Two `BatchMode` read-only checks ended with `Connection closed by 23.80.89.173 port 22` before any command ran. No remote write occurred.
