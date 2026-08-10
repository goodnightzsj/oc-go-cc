# Analytics and Dashboard Correctness

## Goals

- Preserve raw input, cache-read, cache-creation, and prompt-total token meanings across storage, APIs, and UI.
- Record cache usage for non-streaming requests.
- Make History rows and detail dialog keyboard accessible with correct focus restoration.

## Non-Goals

- Replace the existing chart implementation or frontend stack.
- Redesign unrelated dashboard sections.

## Constraints

- Keep existing JSON fields where possible and add an explicit `prompt_tokens` field for display totals.
- Use native DOM, ARIA, and existing styles; add no dependency.
- Every token-contract change needs a focused Go test.

## Done-When

- [ ] Provider responses include cache token fields and tests prove aggregation.
- [ ] History exposes raw input and prompt total without ambiguous double counting.
- [ ] Non-streaming requests persist cache usage.
- [ ] History rows and modal work by keyboard and restore focus.
- [ ] Storage, handler, GUI, and JavaScript checks pass.
