# Progress Log

## Context Recovery Block

- **Current milestone**: Complete
- **Current status**: DONE
- **Last completed**: Child #11 - Upstream remediation, secret audit, and UI documentation
- **Current artifact**: `SUBTASKS.csv`
- **Key context**: All 11 children are complete. Upstream PR #133 contains only the #51/#131 protocol and model fixes; the fork retains its separate request-history and dashboard work.
- **Known issues**: Upstream #54 lacks request identity evidence and #30 lacks a localization acceptance contract; neither was changed speculatively.
- **Next action**: None.

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

- **Status**: DONE
- **Key context**: The provider snapshot corrected primary History once and remains invisible; future proxy traffic follows the normal request path.
- **Validation**: Full local gates pass. Production public APIs and the existing Edge session verify rolling 60-second `0 RPM / 0 TPM` when idle, without changing the retained 1,390 requests, on `4f2ce10`.
- **Next step**: None.

## Child 11: Fix upstream issues, audit secrets, document UI, and submit PR

- **Status**: DONE
- **Started**: 2026-08-11 15:28
- **Completed**: 2026-08-12 12:57
- **Key context**: PR #133 is open and mergeable with a successful automated review; it contains one upstream-based commit and only the intended protocol/model files.
- **Validation**: Full Go tests, vet, formatting, JavaScript syntax, diff checks, secret scans, screenshot inspection, fork push, PR scope, and issue replies all verified.
- **Next step**: None.

## Session Start

- **Date**: 2026-08-08
- **Task name**: `finish-claude-work`
- **Task dir**: `.codex-tasks/20260808-finish-claude-work/`
- **Environment**: Go / embedded HTML-CSS-JavaScript / SQLite / systemd
