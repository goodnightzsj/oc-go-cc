# Progress Log

## Context Recovery Block

- **Current milestone**: Child #10 - correct production request truth and finish the data experience
- **Current status**: IN_PROGRESS
- **Last completed**: Child #10 prior release review
- **Current artifact**: `SUBTASKS.csv`
- **Key context**: Production `d0d8fa5` has the complete 1,390-row provider snapshot, but the primary History table still contains 1,062 mixed-quality rows and `$24.56317345`.
- **Known issues**: Child #10 is reopened for one-time request correction, source-neutral Analytics, removal of recent usage, and a fuller Sub2API-informed chart pass; the unrelated remote `.ace-tool/` directory remains untouched.
- **Next action**: Execute child #10 tasks #6-#10 in order.

## Child 1: Finalize storage overlay regression

- **Status**: DONE
- **Completed**: 01:28
- **Validation**: Related package tests, `go vet ./...`, formatting, and diff checks passed.
- **Next step**: Child 2 - Security and streaming P0 findings.

## Child 2: Verify and fix security and streaming P0 findings

- **Status**: DONE
- **Completed**: 01:37
- **Validation**: GUI, config, and transformer tests plus vet, formatting, and JavaScript syntax checks passed.
- **Next step**: Child 3 - Dashboard correctness.

## Child 3: Fix verified analytics and dashboard P0 findings

- **Status**: DONE
- **Completed**: 01:54
- **Validation**: Storage, GUI, and handler tests plus vet, formatting, and JavaScript syntax checks passed.
- **Next step**: Child 4 - Scoped sub2api presentation port.

## Child 4: Port useful sub2api presentation patterns

- **Status**: DONE
- **Completed**: 02:08
- **Validation**: GUI tests, vet, formatting, JavaScript syntax, and diff checks passed.
- **Next step**: Child 5 - OpenCode usage reconciliation.

## Child 5: Reconcile costs with OpenCode usage

- **Status**: DONE
- **Completed**: 03:06
- **Validation**: Existing authenticated Edge capture yielded 290 post-baseline requests and `$0.44185497` provider cost; proxy and provider token aggregates match (`458090` input, `114821` output, `123418752` cache read). No credentials or request content were persisted.
- **Next step**: Child 6 - Full validation, commit, push, deploy, and smoke-test.

## Child 7: Harden SQLite secret persistence

- **Status**: DONE
- **Started**: 2026-08-10
- **Key context**: The original SQLite review explicitly left `providers.api_key` plaintext persistence for later documentation or encryption; current code still writes it and does not enforce an owner-only database mode.
- **Validation**: SQLite-loaded catalog metadata no longer contains API keys; legacy values are scrubbed; database/WAL/SHM modes are tested at `0600`; storage, catalog, router, GUI tests and focused vet/format checks pass.
- **Next step**: Child #8 - fresh OpenCode usage capture and reconciliation.

## Child 8: Recapture and reconcile OpenCode request costs

- **Status**: DONE
- **Key context**: Fresh authenticated Edge capture contains 1,390 sanitized rows and `$3.03965577`; deterministic matching and explicit cost provenance are implemented and tested.
- **Validation**: Production dry run/apply/second dry run were 290 exact with zero ambiguous/missing/conflicting; provider total is `$0.44185497`.

## Child 9: Complete sub2api-informed Analytics and History design

- **Status**: DONE
- **Key context**: The complete Analytics/History hierarchy, dense filters, responsive behavior, and cost provenance are implemented; static and automated tests pass.
- **Validation**: Production Edge desktop/mobile screenshots and History keyboard/filter checks pass; model/provider charts both total `124.0M`.

## Child 6: Validate, deploy, correct production data, and smoke-test

- **Status**: DONE
- **Validation**: Full local checks pass; commits through `aecbe1c` are pushed; final production release, health, GUI/API, SQLite permissions, exact costs, and Edge workflows pass.

## Child 10: Complete Sub2API parity, custom controls, and refresh behavior

- **Status**: IN_PROGRESS
- **Key context**: The provider snapshot must now correct primary History once, then remain an invisible implementation detail while future proxy traffic follows the normal request path.
- **Validation**: Pending transactional sync tests, complete local gates, production backup/dry-run/apply, API checks, and public-domain Edge desktop/mobile validation.
- **Next step**: Child task #6 - reference and contract re-audit.

## Session Start

- **Date**: 2026-08-08
- **Task name**: `finish-claude-work`
- **Task dir**: `.codex-tasks/20260808-finish-claude-work/`
- **Environment**: Go / embedded HTML-CSS-JavaScript / SQLite / systemd
