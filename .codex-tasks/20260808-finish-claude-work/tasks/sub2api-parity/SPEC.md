# Sub2API Parity, Controls, And Refresh

## Goal

- Use imported OpenCode usage as the current authoritative Analytics baseline, complete the useful Sub2API dashboard/usage-record insights, replace browser-default visible controls with themed accessible controls, localize request details, and restore browser refresh shortcuts.

## Non-Goals

- Copy Sub2API billing, balance, API-key, subscription, IP-geolocation, or tenant-management features.
- Infer unavailable request fields or merge account-wide provider rows into proxy-owned request history.
- Add a frontend framework or chart dependency.

## Constraints

- Keep the existing embedded Go/SQLite/HTML/CSS/JavaScript/SVG stack.
- Label OpenCode as the authoritative Analytics source; keep incomplete local History scoped to request details and filtered local summaries.
- Provider snapshots retain only sanitized time/model/provider/plan/token/cost fields.
- Preserve keyboard focus, Escape, reduced motion, responsive layout, and existing deployment rollback gates.

## Acceptance

- Request detail content is Chinese in Chinese mode and uses the project theme.
- Visible select/date-range/dialog surfaces are custom-themed and keyboard accessible.
- Browser `Cmd/Ctrl+R`, hard reload, F5, and toolbar reload are not intercepted by application JavaScript.
- Analytics exposes the `$3.03965577` OpenCode account billing and 1,390 requests as the current headline source of truth.
- Model, platform/provider, scenario/plan, token trend, recent usage, and History summary views are present and production-validated.
