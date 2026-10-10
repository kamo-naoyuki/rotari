# Plan: Recent Activity View in the Web UI

**Created:** 2026-10-11
**Status:** Proposed; open decisions below need the user's answer before Phase 3
**Related:** [Shared experiment record](../2026-10-10-shared-experiment-record/plan.md) (candidate "marking whether a run was started by an agent")

## Purpose

Two situations need a quick answer to "what has rotari been used for lately,
and where":

- **Coming back after a break.** The user forgets which base directory and
  which project they last ran jobs in, and from which working directory
  (checkout) they ran them.
- **Following a code agent.** An agent runs jobs on its own. Afterwards it is
  hard to see which base directories and projects it used, and which runs were
  its.

The Web UI today starts from one selected basedir and drills down by project
and run. Nothing shows activity across all registered basedirs on one screen,
and nothing tells an agent's runs from a person's.

## What exists today

Checked at `56fee209`.

- `rotari runs` lists active and recently finished runs across all known state
  directories (basedir registry, then run registry), with `--since` (default
  `1d`) and `--json`. Its collector (`collectRunRows`, `readRunRow`) lives in
  [cmd/rotari/lists.go](../../cmd/rotari/lists.go), so the Web server cannot
  call it.
- The Web server's `/api/state` lists registered basedirs but loads projects
  and runs only for the selected basedir; expanding another basedir reads only
  its project names. `/jobs` (`internal/joblist`) lists recent attempts in one
  basedir.
- Each run records, in `context.json`, its working directory (`cwd`), host,
  and `launch_origin` (the parent process of the CLI command; absent for runs
  started from the Web UI or MCP). `sources.json` records the git/jj revision
  per working directory (RUN-15). Runs can carry notes (RUN-16).
- Nothing records whether a run was started by a person or an agent. The
  shared-experiment plan decided not to record authorship of notes, because
  detecting one agent's environment variable (`CLAUDECODE=1`) covers only that
  agent and mislabels a person typing in its terminal.

## Scope

1. A Web **Activity** page across all registered basedirs, built around a
   month calendar and a one-day timeline (see [Page design](#page-design)).
   Clicking a run opens its existing run page in its basedir.
2. The same facts from the CLI, so an agent (which cannot use the Web UI) and
   a person in a terminal get the same answer: `rotari runs` gains the working
   directory and the starter (below) in `--json`, and a column where it fits.
3. Recording **how a run was started**, so the page can show and filter
   agent runs.

## Non-goals

- A dashboard of job-level metrics (durations, resource use). The page shows
  runs; the existing run and jobs pages show jobs.
- Detecting agents by heuristics (process tree, terminal, one vendor's
  environment variable).
- Recording an agent's transcript or session.

## Approach

### Phase 1: one run-list collector

Move `collectRunRows` / `readRunRow` out of `cmd/rotari` into an internal
package (for example `internal/runlist`, beside `internal/joblist`) that
returns typed rows, not display strings. `rotari runs` renders them; the Web
server serves them. This keeps one rule for which runs are listed, their
status, and their order (one rule, one implementation). Pure refactoring:
`conformance/` must pass unchanged.

Add to each row the facts the page needs, read from files the run already
has: working directory and host (`context.json`), source revision labels
(`sources.json`), and whether notes exist. Bound the cost: skip reading a
run's files when its directory's modification time is older than the window,
except for active or interrupted runs, which `runs` always lists.

### Phase 2: the Activity page

- New `GET /api/activity?from=TIME&to=TIME` returning the Phase 1 rows of
  runs that overlap the range, across all registered basedirs, read-only.
  Ranges are capped (about 45 days, enough for a month view); a missing,
  malformed, reversed, or oversized range is rejected with 400 rather than
  clamped silently.
- New page `/activity`, linked from the sidebar, laid out as in
  [Page design](#page-design). Drawn with plain SVG/DOM in the style of the
  existing chart code in `internal/webui/assets`; no new dependency.
- Basedirs that are registered but missing are listed as missing, as
  `basedirs` does, not dropped silently.
- Static export: the page shows the exported basedir only and says so, rather
  than an empty or partial cross-basedir view.

### Page design

The page has two parts that share one selected day: a month calendar on top
and that day's timeline below. Both have their own URL
(`/activity?month=2026-10`, `/activity?day=2026-10-11`) so a view can be
bookmarked and the browser's back button works.

```text
 ◀ October 2026 ▶                                   [Today]  actor: [all ▾]
 ┌──────┬──────┬──────┬──────┬──────┬──────┬──────┐
 │ Mon  │ Tue  │ Wed  │ Thu  │ Fri  │ Sat  │ Sun  │
 │  5   │  6   │  7   │  8   │  9   │ 10   │ 11   │
 │●● 3  │      │● 1   │●●● 9 │●● 4  │● 12  │●  2  │   ● = project colour
 │      │      │      │  ✕2  │      │  ✕1  │  ▶   │   ✕ = failed runs, ▶ = active
 └──────┴──────┴──────┴──────┴──────┴──────┴──────┘
 Sun 11 Oct                                     0    6    12    18    24
 basedir-a / train      ███ try1 ✓   ██████ sweep ✕
 basedir-a / eval                         ██ eval-fix ✓
 basedir-b / default                                   ▓▓▓▓▓▓▓▶ (running)
```

- **Calendar.** Each day cell shows the run count and up to three project
  colour dots (then `+N`); failed and active runs get a mark, so colour is
  never the only signal. Cell shading grows with the time spent running, as
  a heatmap. Clicking a day selects it; a day with one run also offers a
  direct link to that run.
- **Day timeline.** The x-axis is 00:00–24:00 of the selected day in the
  browser's time zone; one lane per (basedir, project). Each run is a bar from
  start to finish, or to now while active, coloured by result. A run crossing
  midnight is clipped on both days with an arrow at the cut. A hover card
  shows the run name, status, duration, working directory, source revision,
  actor, and the first line of its notes; clicking a bar opens the run page.
  The axis can zoom to the busy hours of the day.
- **Coming back after a break.** When the page opens on a day without runs,
  it selects the most recent day that has runs and says so, and the calendar
  shows that month. Below the timeline, a short list of projects with their
  last run time, working directories, and latest source revision answers
  "where was I working?".
- **Following an agent.** The actor filter (Phase 3) narrows both the
  calendar and the timeline; unlabelled runs stay visible under "all".
- **Colours.** Each project gets a stable colour derived from its basedir and
  name, so it keeps the same colour across days and reloads; status uses the
  existing result colours. Both work in light and dark themes.

### Phase 3: how a run was started

Record a `started_by` fact in `context.json` at `Begin`:

- `interface`: `cli`, `web`, or `mcp`, always recorded; it is known exactly.
- `actor`: an optional free-form label taken from `ROTARI_ACTOR` (and a
  `--actor` option on `run` / `retry` for one-off use). An agent is marked by
  setting the variable in its own configuration, for example
  `"env": {"ROTARI_ACTOR": "claude-code"}` in Claude Code's settings, or in the
  shell profile it starts with. rotari never guesses.

Shown on the Activity page (colour or marker per actor, filter by actor), the
run page, `show`, `runs --json`, and the MCP run summary. A run with no actor
is shown as unlabelled, not as "human".

## Open decisions (for the user)

1. **Agent marking.** Is an explicit opt-in label (`ROTARI_ACTOR`) acceptable,
   given that it only works once each agent's configuration sets it? The
   alternative, detecting known agents' environment variables, was rejected
   for notes; this plan proposes the same reasoning for runs.
2. **Opening view**: the current month with today selected, falling back to
   the latest day with runs, as proposed; or a week view as a third mode.
3. **Grouping key**: lanes by (basedir, project) as proposed, or by working
   directory, which is closer to "which checkout was I in"?

## Contracts and documentation

- New WEB rule for the Activity page and API (cross-basedir listing, window
  validation, missing basedirs shown), with a conformance test.
- A RUN rule for `started_by` (interface always recorded; actor only from the
  explicit setting), with conformance through CLI, Web, and MCP starts.
- `docs/ARCHITECTURE.md` (new package, Web server role), the Web UI guide,
  `docs/CONFIGURATION.md` (`ROTARI_ACTOR`), and `contracts/README.md`.

## Validation

- Phase 1: `go test ./cmd/rotari ./internal/...` and `go test ./conformance`
  pass without edits to conformance.
- Phase 2: API tests with a fixture of two basedirs, several projects, runs
  inside and outside the range, a run crossing midnight, an active run, an
  interrupted run, and a missing basedir; the calendar and timeline checked
  in a browser in both themes and at phone width.
- Phase 3: a table test starting runs through CLI, Web, and MCP, with and
  without `ROTARI_ACTOR`, expecting the recorded `started_by` in each.
- `scripts/check.sh` before each commit.
