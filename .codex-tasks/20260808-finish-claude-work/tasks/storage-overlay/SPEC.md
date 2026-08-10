# Storage Overlay Regression

## Goals

- Preserve `storage.DefaultConfig` values when configuration supplies only selected storage fields.
- Preserve an explicit `wal_enabled: false`.

## Non-Goals

- Change storage defaults or configuration file locations.

## Constraints

- Keep merge semantics in one shared storage helper.
- Add no dependencies.

## Deliverables

- Overlay implementation at both storage construction call sites.
- Focused regression tests for partial, empty, and explicit-false overlays.

## Done-When

- [ ] Focused tests pass.
- [ ] All callers use the shared overlay path.
- [ ] Package tests, vet, formatting, and diff review pass.
