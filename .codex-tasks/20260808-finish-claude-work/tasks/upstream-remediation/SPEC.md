# Upstream Remediation, Secret Audit, and PR

## Task Shape

- **Shape**: `single-full`

## Goals

- Fix upstream issue #51 by emitting a protocol-valid thinking signature sequence and a single terminal message delta.
- Fix upstream issue #131 by resolving uniquely matching model identifiers case-insensitively while preserving exact custom identifiers.
- Audit tracked files and Git history for credentials, private keys, tokens, and sensitive local data.
- Add sanitized screenshots of the current public UI to the English and Chinese README files.
- After the audit passes, submit a focused upstream PR and reply to the issues fixed by that PR.

## Non-Goals

- Add speculative request deduplication for issue #54.
- Implement an unspecified additional locale for issue #30.
- Merge the fork's dashboard, SQLite history, reconciliation, deployment, or other fork-only architecture into upstream.
- Rewrite Git history or rotate credentials without separate authorization.
- Investigate the intermittent Edge right-side strip.

## Constraints

- Preserve exact model identifiers before attempting case-insensitive canonicalization; ambiguous matches must not be selected arbitrarily.
- Keep the stream change protocol-focused and covered by exact SSE sequence tests.
- Use only the user's existing Edge session for screenshots and expose no credentials, request content, or private identifiers.
- Do not write to the upstream repository or its issues until the sensitive-information audit is clean.
- Keep all implementation dependency-free and compatible with the existing Go and embedded frontend stack.

## Environment

- **Project root**: `.`
- **Language/runtime**: Go / embedded HTML-CSS-JavaScript
- **Package manager**: Go modules
- **Test framework**: Go testing package
- **Build command**: `go build ./...`
- **Existing test count**: determined by package test execution

## Risk Assessment

- [x] External dependencies (GitHub and the existing Edge session) are available but all writes are gated.
- [x] Streaming event order and model routing are public behavioral contracts requiring regression coverage.
- [x] Screenshot files are bounded and will be inspected before tracking.
- [x] Full test execution is expected to fit the existing project validation path.

## Deliverables

- Stream transformer fix and regression tests for #51.
- Router/catalog fix and regression tests for #131.
- Recorded sensitive-information audit with redacted evidence only.
- Sanitized UI screenshot assets and bilingual README updates.
- Focused upstream branch, PR, and issue replies.

## Done-When

- [ ] Focused and full validation pass.
- [ ] Sensitive-information audit reports no active secret exposure in the proposed changes or repository history.
- [ ] README screenshots are current, legible, and contain no sensitive information.
- [ ] Upstream PR contains only generally applicable fixes and documents fork differences without proposing their wholesale merge.
- [ ] Issues #51 and #131 link to the submitted PR.

## Final Validation Command

```bash
go test ./... -count=1 && go vet ./... && test -z "$(gofmt -l cmd internal pkg)" && node --check internal/gui/assets/app.js
```
