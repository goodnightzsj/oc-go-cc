# Scoped sub2api Presentation Port

## Goals

- Port the remaining high-value dashboard interaction patterns that fit the existing embedded frontend.
- Keep existing analytics visible during background refreshes and ignore stale responses.
- Add a user-controlled, persisted analytics refresh interval that pauses off-tab or while hidden.
- Make donut legends keyboard-expandable with useful numeric detail.

## Non-Goals

- Add Chart.js, xlsx, virtual scrolling, WebSocket, tenant/user/account views, or frontend pricing logic.
- Add a 200-minute health strip without an accurate aggregate data contract.

## Constraints

- Native HTML/CSS/JavaScript and existing analytics response fields only.
- No new backend endpoint or dependency.
- Preserve reduced-motion and keyboard accessibility.

## Done-When

- [ ] Only the latest analytics request can update the UI.
- [ ] Refreshing preserves rendered data and initial empty/error states stay explicit.
- [ ] Off/5s/30s/60s auto-refresh is persisted and pauses when inactive.
- [ ] Model/provider legend rows expand by keyboard and show available metrics.
- [ ] GUI tests and JavaScript syntax checks pass.
