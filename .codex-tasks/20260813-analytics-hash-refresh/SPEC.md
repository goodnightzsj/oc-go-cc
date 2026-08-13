# Analytics hash refresh

## Goal

Ensure a direct browser refresh of `#analytics` initializes the analytics date range before loading data, so KPI values render instead of remaining `--`.

## Scope

- Remove the delayed analytics initialization race in the embedded frontend.
- Add a focused regression contract test.
- Deploy the committed fix to production and verify it on the public domain with the existing Edge tab.

## Non-goals

- Change analytics API behavior or data calculations.
- Modify unrelated deployment-script work already present in the worktree.
- Create a separate browser session.

## Acceptance

- Analytics initializes synchronously before the queued hash activation runs.
- Focused and repository tests pass.
- Production serves the new frontend build.
- Directly refreshing `https://opencode.9962510.xyz/#analytics` renders KPI values rather than `--`.

## Risks

- Production deployment may encounter unrelated remote worktree changes; preserve them and stop if they overlap the fix.
- Browser caching may obscure the new asset; validate the served build identifier and perform a direct refresh.
