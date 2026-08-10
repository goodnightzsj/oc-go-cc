# OpenCode Usage Reconciliation

## Goals

- Use the authenticated local Edge session to read the OpenCode workspace usage page.
- Capture current subscription consumption and representative per-request costs without exposing authentication material.
- Compare post-baseline proxy analytics with provider-reported usage and document explainable differences.

## Non-Goals

- Read or persist cookies, bearer tokens, API keys, or private request content.
- Recompute pre-baseline dirty rows.
- Automate OpenCode account actions or modify provider data.

## Constraints

- Run jshook registry inspect and sync before browser attachment.
- Store only sanitized aggregate facts or request IDs/timestamps needed for reconciliation.
- Stop if the authenticated Edge session is unavailable rather than bypassing login.

## Done-When

- [ ] Current subscription usage is observed or the external blocker is proven.
- [ ] At least a small set of provider request costs is compared with local post-baseline records when available.
- [ ] No credential or request-content material appears in task artifacts or output.
