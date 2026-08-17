# Task Specification

## Task Shape

- **Shape**: `single-full`

## Goals

- Finish the unresolved work on `prune/ponytail-audit` before it can be considered for `main`.
- Make catalog imports atomically replace stale providers and models.
- Validate the consolidated GitHub Actions release workflow as far as local tooling allows.
- Reorganize the unpublished branch into reviewable commits without changing `main`.

## Non-Goals

- Merge into `main`, push, open a PR, or deploy without a separate user request.
- Add dependencies to the repository.
- Rework unrelated behavior already implemented by the Claude session.

## Constraints

- Preserve public behavior except for the already documented Claude-session changes and the catalog replacement fix.
- Keep the original unpublished commit recoverable through a backup branch.
- Preserve a clean, buildable state at every final commit boundary.

## Environment

- **Project root**: `/Users/zsj/code/program/oc-go-cc`
- **Language/runtime**: Go 1.25, vanilla embedded frontend, GitHub Actions
- **Package manager**: Go modules / npm only for existing frontend tooling
- **Test framework**: Go `testing`
- **Build command**: `go build ./...`

## Risk Assessment

- [x] Data integrity: catalog replacement must be atomic and delete children before parents.
- [x] Breaking changes: current branch already contains intentional API/config behavior changes; no additional expansion is allowed.
- [x] History rewrite: branch is unpublished and will be backed up before reorganization.
- [x] CI validation: GitHub-hosted execution cannot be reproduced fully locally.

## Deliverables

- Catalog replacement implementation and regression test.
- Local workflow validation evidence.
- Reviewable unpublished commit sequence based on `main`.

## Done-When

- [ ] A second catalog import removes providers and models absent from the new snapshot.
- [ ] Focused tests, full tests, vet, build, formatting, and workflow validation pass.
- [ ] The branch is split into coherent commits and the original commit remains recoverable.
- [ ] Worktree is clean and `main` remains at `e9e4c7c`.

## Final Validation Command

```bash
go test ./... && go vet ./... && go build ./... && test -z "$(gofmt -l cmd internal pkg)"
```
