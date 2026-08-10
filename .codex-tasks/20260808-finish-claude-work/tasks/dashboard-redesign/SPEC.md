# Analytics and History Design Completion

## Goals

- Complete the sub2api-informed operational dashboard and usage-record experience in the existing embedded stack.
- Make request cost provenance, token composition, failures, and provider/model comparisons easy to scan and drill into.
- Keep filtering, refresh, keyboard, responsive, and empty/error states production-ready.

## Design Direction

- Quiet industrial operations console with cool tinted neutrals and restrained indigo focus.
- Dense unframed data bands, tabular numbers, strong state colors, and progressive disclosure.
- Fixed dashboard typography, 4px spacing rhythm, 150-250ms transform/opacity transitions, reduced-motion support.

## Non-Goals

- Port sub2api backend, tenancy, billing administration, or framework dependencies.
- Replace SVG charts or the dependency-free embedded frontend.

## Done-When

- [ ] Analytics provides clear trends, comparisons, cost provenance, and meaningful drill-downs.
- [ ] History provides ergonomic filters, sortable exact/estimated cost, request details, pagination, and export.
- [ ] Desktop and mobile screenshots show no overlap, clipping, or blank charts.
- [ ] Keyboard, reduced-motion, JavaScript syntax, and GUI tests pass.
