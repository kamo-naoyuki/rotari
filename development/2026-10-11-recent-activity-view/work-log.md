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
