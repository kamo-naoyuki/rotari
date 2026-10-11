# Work Log: Recent Activity View in the Web UI

Entries group cohesive changes. Times are Git commit times.

## The plan

- `3588768f` (2026-10-11 01:51:56 +0900): the plan.

**Change:** Added [plan.md](plan.md).

**Reason:** the user wants to see at a glance which basedirs, projects, and working directories were used recently, both after a break and to follow what a code agent ran.

**Plan impact:** proposes a shared run-list collector (moved out of `cmd/rotari/lists.go`), a cross-basedir Web Activity page, and recording how a run was started (`interface` always, `actor` only from an explicit `ROTARI_ACTOR`). Three decisions are left to the user: agent marking, default window, and lane grouping.

**Validation:** documentation only; no tests run.

**Remaining:** the user's answers to the open decisions; Phases 1–3.

## Calendar and day-timeline design

- `d01eae7b` (2026-10-11 02:16:20 +0900): page design.

**Change:** [plan.md](plan.md) gains a "Page design" section and a range-based API (`/api/activity?from=&to=`, capped, invalid ranges rejected) in place of the `since` windows.

**Reason:** the user cares about how the page looks and asked for a calendar whose days lead to runs, and a one-day view where each run's running span is a clickable timeline bar.

**Plan impact:** the page is a month calendar (run counts, project colour dots, failed/active marks, heatmap shading) above the selected day's timeline (lanes per basedir/project, bars from start to finish, hover card, click to the run page). Opening falls back to the latest day with runs. Open decision 2 is now the opening view instead of the default window.

**Validation:** documentation only; no tests run.

**Remaining:** the user's answers to the open decisions; Phases 1–3.

## Decisions and the many-project layout

- `df5c91e0` (2026-10-11 02:21:04 +0900): decisions.

**Change:** [plan.md](plan.md) replaces "Open decisions" with "Decisions" and adds "Many projects" to the page design.

**Reason:** the user accepted `ROTARI_ACTOR` and the opening view, and wants lanes across basedirs but was unsure how the page behaves with many projects.

**Plan impact:** lanes are one per (basedir, project) across basedirs, only for projects active on the selected day, grouped under collapsible basedir headers, with about 12 lanes before "+N more" folding. Lanes never overlap because a project runs one run at a time. All open decisions are closed.

**Validation:** documentation only; no tests run.

**Remaining:** Phases 1–3; tune the lane threshold on real data.

## Mockup and colour decisions

- `01652576` (2026-10-11 02:40:01 +0900): colour decisions and the mockup link.

**Change:** [plan.md](plan.md) rewrites "Colours" in the page design, adds a colour decision, and links the sample-data mockup (a private artifact, not in the repository).

**Reason:** the user wanted to check the look with sample data before implementation.

**Plan impact:** bars stay coloured by result; project identity uses eight colours given to the month's busiest projects, the rest grey, computed from all runs so the actor filter does not repaint. The page keeps the Web UI's dark theme.

**Validation:** the mockup's script passed `node --check`; it was not opened in a browser by the agent. The user reviewed the published page and accepted the look.

**Remaining:** Phases 1–3.

## Themes from the visual refresh

- `41122c7b` (2026-10-11 02:47:13 +0900): themes.

**Change:** [plan.md](plan.md)'s "Colours" now says the page uses the Web UI's tokens and both themes.

**Reason:** the user chose a light theme in the [Web visual refresh](../2026-10-11-web-visual-refresh/plan.md) plan.

**Plan impact:** the Activity page depends on that plan's Phase 1 tokens; its colours must work on both backgrounds.

**Validation:** documentation only; no tests run.

**Remaining:** Phases 1–3.

## Phase 1: one run-list collector

- `a54fb30f` (2026-10-11 22:17:40 +0900): the collector moved into `internal/runlist`.
- `d2e8870f` (2026-10-11 22:27:25 +0900): run details.

**Change:**
- `internal/runlist`: `Collect` (which runs are listed, their lifecycle, client status, and order, as recorded values), `Recent` (the `runs` window), and `DescribeScope`, moved from `cmd/rotari/lists.go` with their tests; `rotari runs` formats the rows. The one visible difference: a run whose summary has no start time shows `-` instead of an empty cell.
- `runlist.ReadDetails`: working directory and host (`context.json`), source revision labels (`sources.json`), and the run's own notes (count and the first line of the earliest), read only for runs a caller shows; an unreadable file is an error naming the run. `rotari runs --json` gains `cwd`, `host`, `sources`, and `note_count` (`docs/INSPECT.md`).
- `docs/ARCHITECTURE.md`: the package table lists `internal/runlist`.

**Reason:** Phase 1 of the plan. The plan's "skip runs whose directory is older than the window" was not applied to listing, because `runs` lists old incomplete runs; the cost is bounded instead by reading details only for the runs shown.

**Plan impact:** Phase 1 done. Phase 2 (`/api/activity` and the page) can call `runlist.Collect` and `ReadDetails` across registered basedirs.

**Validation:** `go test ./internal/runlist ./cmd/rotari`, `go test ./conformance/...` unchanged and passing after `a54fb30f`, pre-commit, and the full `scripts/check.sh` before each commit.

**Remaining:** Phases 2–3.

## Phase 2: the Activity API and page

- `a5c3d379` (2026-10-11 23:08:51 +0900): `/api/activity`.
- `9ddee467` (2026-10-11 23:37:59 +0900): the Activity page.

**Change:**
- `runlist.Run.Span` and `Overlapping`: a run spans its start to its finish, to now while running, and only its start when it recorded no finish.
- `GET /api/activity?from=&to=` (`internal/webui/activity.go`): runs of every registered basedir meeting the range, with `runlist.ReadDetails` facts and `ends_at` (the span end); missing basedirs marked, unreadable ones carry their error; missing, malformed, reversed, or over-45-day ranges rejected with 400. New contract rule WEB-10 with `TestActivityListsRunsAcrossRegisteredBasedirs` (conformance) and `TestActivityListsRunsAcrossBasedirs`, `TestActivityRejectsBadRanges`.
- `web_app_activity.js`: the page as designed in the plan and mockup (month calendar, day timeline grouped by basedir, Where you worked, hover card, fallback to the latest day with runs, `?day=` / `?month=` in the URL). New tokens `--c6`–`--c8` complete eight project colours, checked with the dataviz palette validator in both themes (the adjacent CVD separation of `--c6`/`--c7` is in the 6–8 band, acceptable because project colours always sit beside names). Views gains Activity in both sidebars.
- Static export: `staticActivity` embeds every run of the exported basedir (`__ROTARI_STATIC_ACTIVITY__`), `activity/index.html` is written, and the static router treats `/activity/` as a top-level page like `/search/`; the page says it shows one basedir.
- Tests: `TestActivityPage` (jsdom), `TestStaticExportIncludesActivity`, `TestSpanAndOverlapping`; `TestWebHTMLRendersState` expects Activity before History search under Views.
- Docs: WEB-10 and the page paragraph in `contracts/05-web-assets-and-static-export.md`, `contracts/README.md`, `docs/FAQ.md` (Where did I run jobs recently?), `docs/ARCHITECTURE.md` (the activity API).

**Plan impact:** Phase 2 done. Deviations: the actor filter waits for Phase 3; "Where you worked" covers the loaded calendar weeks up to the selected day rather than a fixed 30 days; `scripts/web-structure.sh` does not compare the Activity page, whose static view intentionally shows one basedir.

**Validation:** a live server over two basedirs (the demo state and a five-job run) checked in light, dark, and at 390px (no sideways page scroll); the static export opened from disk shows the exported basedir's runs and its notice. `go test ./internal/runlist ./internal/webui ./conformance/05-web`, `TestContractStatus`, `npm run typecheck`, pre-commit, and the full `scripts/check.sh` passed before each commit.

**Remaining:** Phase 3 (`started_by` and the actor filter).
