# Progress Log

## Context Recovery Block

- **Current milestone**: #4 - Run focused validation
- **Current status**: DONE
- **Last completed**: #3 - Enforce owner-only database permissions
- **Current artifact**: `TODO.csv`
- **Key context**: The original SQLite review left plaintext `providers.api_key` persistence unresolved. Configuration remains the expected credential source.
- **Known issues**: `Database.Open` creates the parent directory but does not enforce the database file mode; catalog upserts and loads `api_key` directly.
- **Next action**: Child #8 - capture current OpenCode usage and reconcile exact provider costs.

## 2026-08-10 - Consumer classification

- `internal/catalog` can still parse credentials from a standalone JSON catalog.
- SQLite catalog loading is used for model metadata and selection; `internal/router.resolvedModelToConfig` does not copy catalog credentials into runtime requests.
- Runtime provider authentication remains owned by `internal/config`, so clearing SQLite credentials does not change routing or request authentication.

## 2026-08-10 - Implementation and tests

- Storage catalog records no longer expose or persist `APIKey`; legacy database values are cleared during `Open`.
- Database creation tightens pre-existing files and verifies the main database plus WAL/SHM sidecars are owner-only.
- `go test ./internal/storage ./internal/catalog -count=1` and dependent router/GUI package tests pass.
- Focused vet, formatting, and diff checks pass; child #7 is complete.
