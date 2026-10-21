# Plan: Artifacts and Launch for running jobs

Created: 2026-10-22
Status: Proposed. Nothing implemented. Waiting for agreement on the attempt
rule (Decision 1).

## Goal

Let the Web run page open a job's Artifacts, and launch a viewer on its
working directory, while the job is still running. The main use case is
watching a training run live: TensorBoard on a log directory, or Aim or
MLflow on the tracker store, before the job finishes.

## Background

A run job row's **Artifacts** button, and the **Launch** button added next to
it on 2026-10-22 (751fc26a), appear only when the job has a `result`, that is,
after it finishes (`runJobActions` in
[web_app_core.js](../../internal/webui/assets/web_app_core.js)). Nothing below
the row requires a finished job:

- **The record exists from the start.** `artifactRecorder` discovers an
  attempt's candidates when the attempt is prepared and writes
  `artifacts.json` into the attempt directory once it has started
  (`started` in
  [internal/projectrun/artifacts.go](../../internal/projectrun/artifacts.go)).
  An attempt that never started has no record.
- **The API reads any attempt.** `GET /api/artifacts` (`webArtifacts` in
  [internal/webui/artifacts.go](../../internal/webui/artifacts.go)) takes an
  optional `attempt_id`, checks that it belongs to the job, and lists that
  attempt through `jobstatus.ListArtifacts`. It has no result condition.
  Content routes and `POST /api/launch` resolve entries through the same
  listing (`resolveArtifact`).
- **The CLI already works.** `rotari show -j JOB --artifacts`
  (`showJobArtifacts` in
  [cmd/rotari/show_artifacts.go](../../cmd/rotari/show_artifacts.go)) lists
  any attempt, running or not.

The result condition dates from the first Web Artifacts commit (e316f0a1),
which gave no reason for it. It most likely followed the Log button's
finished branch, which opens the attempt that produced the result. The Log
button's unfinished branch already opens the job's current attempt
(`j.attempt_id`), and is disabled with "Job has not started yet" before
the job starts.

So the gap is one display condition in the Web UI, on one interface. The
CLI and the API already behave as wanted.

## Scope and non-goals

Include:

1. Show Artifacts and Launch on a run job row once the job has started,
   for the same attempt the row's Log opens.
2. Disable both, with the reason, on a job that has not started, as Log is.
3. Tests, contract, and user guide updates for the new rows.

Exclude:

- Changing when discovery runs or when `artifacts.json` is written.
- Refreshing an open Artifacts listing while the job runs. The listing
  observes each path when it is opened; reopening it shows new files.
- The run toolbar's launch buttons for the directory holding the basedir.
  They target the run, not an attempt, and already work while it runs.
- Static exports. They embed each job's listings at export time, running or
  not (`staticArtifactAttempts`), and cannot launch.

## Decisions

### 1. Which attempt a running job's Artifacts opens (to confirm)

**Recommended:** the same attempt as the row's Log, decided in one place:

| Row state                                  | Log and Artifacts open                                  |
|--------------------------------------------|---------------------------------------------------------|
| Finished, executed in this run             | the attempt that produced the result                    |
| Finished, carried from an earlier run      | no attempt; the server follows the carry (unchanged)    |
| Started, not finished                      | the current attempt, `j.attempt_id`                     |
| Older attempt chosen in the row            | that attempt (`selectJobAttemptFields`, unchanged)      |
| Not started                                | disabled: "Job has not started yet"                     |

Alternative: always open the newest attempt for Artifacts, even when Log
shows a finished one. This was rejected because Log and Artifacts would then
show different attempts of the same row, and the row's attempt selector
already reaches any attempt.

Open question for Phase 1: whether a job whose latest attempt is running can
still carry the `result` of an earlier attempt of the same run, for example
after a requeue. If it can, the finished branch would open the old attempt
while the new one runs. Phase 1 settles this from the state code before the
rule is fixed.

### 2. One rule for the row's attempt

Today `runJobActions` computes the Log attempt (`logRun`, `logJob`,
`logAttemptID`, `carried`) and reuses it for Artifacts. The change keeps
that: Artifacts and Launch take the attempt the Log button uses, and only
the condition for showing them changes from "has a result" to "has
started".

The Log button's started test (`submitted_at`, a result, or a running run)
is too broad here. While the run is running, it counts every job as started,
including ones still pending, which have no attempt record. That is
harmless for Log, which follows the output as it appears, but it would offer
Artifacts that can only say "not recorded". Phase 1 checks whether a pending
job carries an `attempt_id` or `submitted_at` in the run detail, and
picks the narrowest fact that means "this attempt has started", most likely
`submitted_at`. If Log should use the same fact, that is a separate change
and goes to `ISSUES.md`. It is not folded in here.

### 3. A started job without a record

Between an attempt's start and its record being written, or for an attempt
whose record failed to write, the listing says `Artifacts: (not recorded)`.
In that case Launch has no `.` row and explains this through
`launchOnWorkingDirectory`'s existing message ("This attempt recorded no
working directory to launch on."). Nothing new is needed; Phase 2 tests it.

## Phases

### Phase 1: Confirm the facts with failing tests

- In [run_jobs_test.go](../../internal/webui/run_jobs_test.go), add rows with
  distinct states: running with `submitted_at` and no result, not started,
  finished, and, if Decision 1's open question holds, a job with a finished
  earlier attempt and a running latest one. Expect Artifacts and Launch on
  the running row with that row's attempt ID, and both disabled with the
  reason on the not-started row. Confirm they fail for the stated reason
  before the fix.
- In the browser launch tests
  ([launch_browser_test.go](../../internal/webui/launch_browser_test.go)), give
  the fixture an attempt with a record and no final result, and check that
  `/api/artifacts` lists it and that Launch opens its `.` choices.
- Settle Decision 1's open question from `internal/jobstatus` and
  `internal/webui/status.go`, and record the answer here.

### Phase 2: Change the row condition

- In `runJobActions`, show Artifacts and Launch when the job has started,
  with the Log button's attempt. Disable them before it starts, with the same
  title as Log.
- Check the variants AGENTS.md lists: plain job, array task, matrix member,
  latest and older attempt (the row's attempt selector), executed and
  carried result. Also check an SSH or scheduler job whose working
  directory is not on the Web host; it must keep the "does not exist on this
  host" explanation.

### Phase 3: Contract, conformance, and guides

- Contract: the run jobs table paragraph in
  [05-web-assets-and-static-export.md](../../contracts/05-web-assets-and-static-export.md)
  ("Artifacts on a finished job, Launch on a finished job") and WEB-5's
  description of the button.
- Conformance: a row in
  [conformance/05-web/artifacts_test.go](../../conformance/05-web/artifacts_test.go)
  that starts a job which waits on a file, lists `/api/artifacts` for its
  running attempt, then lets it finish. This pins the API behavior the UI
  now relies on.
- Guides: [docs/INSPECT.md](../../docs/INSPECT.md) (the Artifacts paragraph
  and "Launching commands on artifacts") and the tracker section's note that
  stores can be opened while a job runs.

## Acceptance

- A running job's row offers Artifacts and Launch for the attempt its Log
  shows; a job that has not started shows both disabled with the reason.
- Finished, carried, and older-attempt rows behave as before.
- `go test ./internal/webui`, `go test ./conformance/05-web`, and
  `scripts/check.sh` pass. The new tests were seen failing before the fix.
