# Work Log: Visual Refresh of the Web UI

Entries group cohesive changes. Times are Git commit times.

## The plan and the mislabelled column

- `1385883c` (2026-10-11 02:44:02 +0900): ISSUES entry for the projects table's Started column shown as "Actions".
- `1da94ca5` (2026-10-11 02:44:03 +0900): the plan.

**Change:** Added [plan.md](plan.md) and an Open item in [ISSUES.md](../ISSUES.md).

**Reason:** after the Activity mockup, the user asked whether the rest of the Web UI could look as good, as the next plan.

**Plan impact:** the survey found that the look is applied by post-render DOM patches and inline styles (351 `.style.*` assignments in `web_app_charts.js`), so the plan puts tokens and a status component first, then makes renderers own their markup before any page redesign. Each phase gets a mockup for the user's approval. Open: light theme, embedded fonts, run page sections open by default.

**Validation:** built `rotari` at `01652576`, created a sample state (two projects, three runs, a failing job), exported it with `rotari web --static-dir`, and screenshotted the projects, project, run, and jobs pages with headless Chrome at 1400×1000; the observations in the plan come from those screenshots and the asset sources. No tests run; documentation only.

**Remaining:** the user's answers; Phases 0–4.
