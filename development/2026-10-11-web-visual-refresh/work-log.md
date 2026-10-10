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

## Decisions

- `2fe9e1a0` (2026-10-11 02:47:13 +0900): decisions.

**Change:** [plan.md](plan.md) replaces "Open decisions" with "Decisions" and widens Phase 1.

**Reason:** the user chose a light theme, embedded fonts, and the proposed run page sections.

**Plan impact:** Phase 1 now also adds the light theme (OS setting, overridable per browser from Settings) and embeds an openly licensed sans and monospace face as a Latin-subset WOFF2 asset with system fallback. The run page opens the job timeline by default and remembers the other sections per browser.

**Validation:** documentation only; no tests run.

**Remaining:** Phases 0–4; choosing the font faces with the Phase 1 mockup.
