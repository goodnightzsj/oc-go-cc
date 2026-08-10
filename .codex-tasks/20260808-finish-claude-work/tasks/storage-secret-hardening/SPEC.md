# SQLite Secret Persistence Hardening

## Goals

- Remove unnecessary plaintext API-key persistence from the SQLite catalog path.
- Enforce owner-only permissions on the database file and SQLite sidecars.
- Preserve catalog routing behavior and existing configuration as the credential source of truth.

## Non-Goals

- Introduce a new encryption dependency or key-management system.
- Change provider configuration formats or rotate credentials.

## Done-When

- [ ] Catalog sync/load works without storing usable provider credentials in SQLite.
- [ ] Newly opened database files are owner-only.
- [ ] Focused storage and catalog tests pass.
