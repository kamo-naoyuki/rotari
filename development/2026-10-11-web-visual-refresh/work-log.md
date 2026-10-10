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

## Language

- `9e7202a8` (2026-10-11 02:50:21 +0900): language decision.

**Change:** [plan.md](plan.md) gains the "Language" decision and a Phase 2 note.

**Reason:** the user asked whether the Web UI should move to TypeScript.

**Plan impact:** assets stay plain JavaScript embedded as they are, so `go build` alone builds a complete binary and no generated JS is committed; JSDoc types checked by `tsc --checkJs --noEmit` in CI, starting with the files Phase 2 rewrites. CI already installs Node and npm dependencies.

**Validation:** documentation only; no tests run.

**Remaining:** adding `typescript` and the check in Phase 2.

## Phase 0: screenshot baseline

- `50fa9860` (2026-10-11 02:59:01 +0900): a run note and a failing array in the static web demo.
- `b165bf90` (2026-10-11 02:59:02 +0900): `scripts/screenshot-web.sh` and `scripts/screenshot-web.mjs`.
- `196a7004` (2026-10-11 02:59:02 +0900): plan update.

**Change:**
- `scripts/generate-static-web.sh` adds `--note` to the demo's retry run and a `shards` project whose six-task array fails with exit codes 2 and 137, so array and failure-cause views have distinct failures. The demo is also what the Pages workflow publishes.
- `scripts/screenshot-web.sh OUTPUT_DIR` runs that demo build, finds Chrome, and calls `scripts/screenshot-web.mjs`, which drives Chrome over the DevTools protocol (Node 22's built-in WebSocket, no new dependency): exact viewports (1400×900, 390×844 mobile), emulated `prefers-color-scheme`, full-page captures up to 8000px, one browser context per capture, and an `index.html` naming pages without run IDs so before/after sets line up.
- `scripts/README.md` lists the script. [plan.md](plan.md) describes Phase 0, adds the phone-width findings, and withdraws the "dark band at the bottom" finding.

**Reason:** Phase 0 of the plan: a way to review each phase's visual change.

**Plan impact:** the first attempt used `chrome --screenshot --window-size`; measuring `innerHeight` showed the viewport is 87px shorter than the window, so the bottom of each image lies outside the page. That explained the dark band and a repeated header in tall captures, so the plan's band finding was an artifact and was removed. The new captures found real phone-width problems instead: a mostly empty 350px top bar, a cut sidebar heading, and header buttons that make the home and project pages 497px wide on a 390px viewport. Not covered yet: an active run and opened modals/sections.

**Validation:** `scripts/screenshot-web.sh` ran to completion three times (the last after formatting the helper with the repository's Prettier): 12 pages, 48 screenshots, exit 0; sample captures inspected (run page with the failing array at desktop, project page at phone width). `bash -n`, `node --check`, and `pre-commit run` on the changed files (shfmt, whitespace) passed. No Go tests run: no Go code changed.

**Remaining:** Phase 1 mockup (tokens, status pill, light theme, font choice).

## Phase 1 mockup and its decisions

- `8e139fd2` (2026-10-11 04:59:07 +0900): font, logo, and column decisions.

**Change:** [plan.md](plan.md) links the Phase 1 mockup (a private artifact: the demo's "Decode shards" run page with tokens, status pills, light/dark/system themes, a Settings menu, and a font switcher) and records three decisions.

**Reason:** the user reviewed the mockup.

**Plan impact:** IBM Plex Sans and Mono are the embedded fonts; the logo is kept as is; tables keep every column, with the command under the job name and same-for-all columns hidden behind a Columns menu. Three mockup faults the user found were fixed before these decisions: Command and other columns were missing, a later revision's table did not render (a column without a value function threw during the auto-hide check; found with jsdom), and the logo had been replaced by a drawn stand-in.

**Validation:** documentation only; no tests run. The mockup was checked in headless Chrome (both themes, desktop) and its table and Columns menu in jsdom.

**Remaining:** whether to keep the summary cards and the note at the top of the run page; Phase 1 implementation.

## Run page top

- `e9036121` (2026-10-11 05:00:29 +0900): decision.

**Change:** [plan.md](plan.md) records that the run page keeps the mockup's notes and four summary cards at the top (Phase 4).

**Reason:** the user accepted the top of the Phase 1 mockup as it is.

**Plan impact:** all mockup questions are closed.

**Validation:** documentation only; no tests run.

**Remaining:** None for this decision.

## Phase 1: tokens, light theme, status pill, fonts

- `6655c9d0` (2026-10-11 05:14:45 +0900): design tokens and the light theme.
- `de9484b7` (2026-10-11 05:21:06 +0900): the System / Light / Dark choice.
- `107c28e1` (2026-10-11 05:29:53 +0900): one status pill and one status-to-tone rule.
- `0291a669` (2026-10-11 05:39:24 +0900): embedded IBM Plex Sans and Mono.

**Change:**
- New `internal/webui/assets/web_tokens.css` (light values on `:root`, dark values under `prefers-color-scheme: dark` guarded by `:root:not([data-theme="light"])` and under `:root[data-theme="dark"]`), served at `/web_tokens.css`, linked from the main template, written beside every static page, and inlined into the jobs page. `web_styles.css`, `web_sidebar_styles.css`, and `web_info_styles.css` lose their `:root` blocks and every colour literal; the chart and table scripts take colours from tokens. `brandIcon` embeds both favicons and the tokens stylesheet shows the one for the theme. The undefined `var(--border)` became `var(--line)`.
- New `web_theme.js`, inlined in `<head>` of both page templates: applies the stored choice before drawing, binds the sidebar `Theme` select, follows other tabs. FAQ entry "Does the Web UI have a light theme?".
- New `internal/webui/status.go` (`statusTones`, `statusTone`, `statusPillClass`, `statusPillHTML`), injected into the page script as `statusTones`; `jobsStateClass`, `jobStatusClass`, and `applyStatusColors` removed. `.status-pill.tone-*` with icons in the shared stylesheet; carried results get a dashed outline; matrix cells colour running and blocked apart.
- New `internal/webui/fonts.go` and `assets/fonts/` (five IBM Latin-1 WOFF2 files, OFL licence, provenance README): `/fonts/` and `/web_fonts.css` on the live server; `fonts/` once at a static export's root with a relative `web_fonts.css` per page directory; `--font-sans` / `--font-mono` tokens used by every font declaration.
- Tests: `TestStylesAndScriptsTakeColoursFromTokens`, `TestWebThemeChoice`, `TestStatusToneMatchesInGoAndJS`, `TestLiveServerServesEmbeddedFonts`, `TestStaticExportWritesFontsOnce`, `TestStylesheetsTakeFontsFromTokens`. Updated for intended changes: the static stylesheet test (tokens moved to their own file), the script syntax test (now one check per `<script>`), the sidebar control list (adds the theme select), the timeline harness (includes the status table), status-class assertions in `TestWebHTMLRendersState`-style checks, `run_status_test.go`, and `TestJobsHTMLStylesStates` (same meanings, new pill classes); `TestJobsStateClassTreatsAcceptedSuccessAsSuccess` became `TestStatusToneTreatsAcceptedSuccessAsSuccess`.

**Reason:** Phase 1 of the plan, as approved in the Phase 1 mockup.

**Plan impact:** Phase 1 done; see the plan's Phase 1 notes for what it turned out to need (semantic mapping instead of a no-op, four status rules merged, IBM's own font files because of the OFL Reserved Font Name). Opened for Phase 2: duplicate `addJobTimeline` / `renderJobTimelineScratch` definitions, and renaming `web_sidebar_styles.css` now that it holds shared components. The Web UI now colours running blue while the CLI keeps yellow.

**Validation:** `go test ./internal/webui` and `scripts/check.sh --short` passed before each commit; pre-commit (prettier, gofmt, whitespace) passed on the changed files. The new theme, parity, and static-font tests were each shown to fail on a deliberately broken implementation (no early theme apply; JS not stripping "..."; a non-relative font path) and to pass when restored. `scripts/screenshot-web.sh` was run after each step and the light and dark captures of the run, project, home, and jobs pages inspected. After the last commit, the full `scripts/check.sh` (vet, tests, and the race detector) passed with exit 0; Go reused cached results for packages this phase did not change.

**Remaining:** Phases 2–4.
