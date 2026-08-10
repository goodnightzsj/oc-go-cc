# OpenCode Provider Usage Correction

## Goals

- Recapture current subscription and per-request usage from the authenticated local Edge tab.
- Match provider usage rows to trustworthy proxy requests deterministically using sanitized fields.
- Persist provider-reported request costs with explicit provenance and correct production rows through a backed-up dry-run workflow.

## Non-Goals

- Store cookies, authorization material, request content, or pre-baseline dirty rows.
- Guess matches when sanitized fields are ambiguous.

## Done-When

- [ ] Current subscription facts and representative request rows are captured without secrets.
- [ ] Exact, ambiguous, missing, and conflicting matches are reported separately.
- [ ] Provider-reported costs are distinguishable from estimates in storage and UI.
- [ ] Production correction is preceded by a database backup and zero-ambiguity dry run.
