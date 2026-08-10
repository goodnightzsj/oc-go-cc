# Sub2API Parity, Controls, And Refresh

## Goal

- Use the imported OpenCode usage snapshot to perform a one-time correction of request history, then complete the useful Sub2API dashboard and usage-record presentation with compact, legible charts and themed Chinese interactions.

## Non-Goals

- Copy Sub2API billing, balance, API-key, subscription, IP-geolocation, or tenant-management features.
- Show provider provenance or describe records as coming from OpenCode in the product UI.
- Keep a separate recent-usage list in Analytics.
- Add a frontend framework or chart dependency.

## Constraints

- Keep the existing embedded Go/SQLite/HTML/CSS/JavaScript/SVG stack.
- Correct existing request history once from the complete sanitized provider snapshot; subsequent proxy requests continue through the normal request-storage path.
- Provider snapshots retain only sanitized time/model/provider/plan/token/cost fields.
- Do not invent unavailable imported fields such as latency, scenario, streaming mode, attempt count, or HTTP status.
- Preserve keyboard focus, Escape, reduced motion, responsive layout, and existing deployment rollback gates.

## Acceptance

- Request detail content is Chinese in Chinese mode and uses the project theme.
- Visible select/date-range/dialog surfaces are custom-themed and keyboard accessible.
- Browser `Cmd/Ctrl+R`, hard reload, F5, and toolbar reload are not intercepted by application JavaScript.
- Analytics and History both expose exactly 1,390 corrected snapshot requests totaling `$3.03965577` before subsequent live traffic.
- Analytics contains no data-source label or recent-usage list.
- Dashboard and History expose useful request, cost, token, model, platform, plan, scenario, and time-series comparisons without misleading single-bar charts.
- Chart tooltips are custom-themed, Chinese, keyboard/touch reachable, and present labels plus exact values.
- Production is validated in Edge only through `https://opencode.9962510.xyz/`.
