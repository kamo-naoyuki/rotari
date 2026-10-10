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
