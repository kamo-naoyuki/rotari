# Work Log: Recent Activity View in the Web UI

Entries group cohesive changes. Times are Git commit times.

## The plan

- `3588768f` (2026-10-11 01:51:56 +0900): the plan.

**Change:** Added [plan.md](plan.md).

**Reason:** the user wants to see at a glance which basedirs, projects, and working directories were used recently, both after a break and to follow what a code agent ran.

**Plan impact:** proposes a shared run-list collector (moved out of `cmd/rotari/lists.go`), a cross-basedir Web Activity page, and recording how a run was started (`interface` always, `actor` only from an explicit `ROTARI_ACTOR`). Three decisions are left to the user: agent marking, default window, and lane grouping.

**Validation:** documentation only; no tests run.

**Remaining:** the user's answers to the open decisions; Phases 1–3.
