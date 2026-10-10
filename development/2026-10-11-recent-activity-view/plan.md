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

1. A Web **Activity** page across all registered basedirs: a time axis with
   one lane per (basedir, project), each run drawn as a bar from start to
   finish (or to now while active) and coloured by result; under it a
   "recently used" list with one row per project: basedir, project, last run
   time, working directories used, latest source revision, run count in the
   window. Clicking a run opens its existing run page in its basedir.
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

- New `GET /api/activity?since=DURATION` returning the Phase 1 rows across all
  registered basedirs, read-only; invalid durations are rejected with 400, as
  `runs --since` rejects them.
- New page `/activity`, linked from the sidebar. The timeline is drawn with
  the existing chart code style in `internal/webui/assets` (no new
  dependency). Window choices: 1d, 7d, 30d; default 7d.
- Basedirs that are registered but missing are listed as missing, as
  `basedirs` does, not dropped silently.
- Static export: the page shows the exported basedir only and says so, rather
  than an empty or partial cross-basedir view.

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
2. **Default window** for the page: 7 days proposed. `runs` keeps `1d`.
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
  inside and outside the window, an active run, an interrupted run, and a
  missing basedir; the page checked in a browser.
- Phase 3: a table test starting runs through CLI, Web, and MCP, with and
  without `ROTARI_ACTOR`, expecting the recorded `started_by` in each.
- `scripts/check.sh` before each commit.
