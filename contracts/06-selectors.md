# Job and project selectors

This note tabulates how commands turn their location and job selectors into
the project, run, and jobs they act on. The resolution rules themselves
(base directory, project, run registry, `latest`) are in
[01-resolution-and-config.md](01-resolution-and-config.md); this note is the
per-command view, and the reference for selector tests.

Representative implementation and tests:

- [internal/resolve/resolve.go](../internal/resolve/resolve.go) and
  [its tests](../internal/resolve/resolve_test.go): locations, run IDs,
  attempt IDs, and job lookup in a queue or run.
- [internal/model/command_selector.go](../internal/model/command_selector.go)
  and [its tests](../internal/model/command_selector_test.go): selecting
  queue commands by job ID, name, stage, matrix, or all.
- [conformance/06-selectors/selector_cases_test.go](../conformance/06-selectors/selector_cases_test.go):
  the tables below as test cases, run through the built binary by
  `TestSelectorTable` in [selector_test.go](../conformance/06-selectors/selector_test.go)
  against the fixture in
  [selector_fixture_test.go](../conformance/06-selectors/selector_fixture_test.go) (see
  [Fixture](#fixture)).

When changing a selector, update this note and add or change its test row in
the same commit.

## Complete IDs

**SEL-1** A run ID or an attempt ID alone resolves the base directory, the project,
and the run (and, for an attempt, the job), through the run registry
(`resolve.ExistingRun` and `resolve.Attempt`). Wherever a command takes one,
as an option or positionally, it resolves them this way, so no base
directory or project option is needed to locate it; an explicit one that
disagrees with the registry is an error. Whether the command then succeeds
depends on the command, such as `unlock`, which also requires the run to be
the locked one. Every other selector (job IDs, names, stages, run names)
depends on the resolved base directory and project, and may not be unique.
Covered by the "complete" rows of `TestPositionalArguments` in
[conformance/06-selectors/positional_test.go](../conformance/06-selectors/positional_test.go).

## Selector forms

| Form | Example | Names |
| --- | --- | --- |
| Base directory | `--basedir/-b DIR`, `ROTARI_BASEDIR` | the state directory |
| Project | `--project-name/-p NAME`, `ROTARI_PROJECT_NAME`, the only project | a project |
| Run ID | `--run-id/-r ID`, `latest` | a saved run; a registered ID also names its base directory and project; `latest` is the project's latest settled run (not active or interrupted), and no name may be `latest` |
| Job ID | `--job-id/-j ID` | a queued command |
| Array task ID | `ID-2` | one task of an array command |
| Attempt ID | `att_<run>-<job>-<n>` | one execution attempt; also names its run |
| Job name | `--job-name NAME` | a named command |
| Array task name | `NAME[2]` | one task of a named array command |
| Group | `--stage STAGE`, `--matrix NAME`, `--all` | every command of a stage, of a matrix (by base job name), or of the queue |
| Result filter | `--failed`, `--unfinished`, `--success` | jobs by their result in the reference run || Filter | `--filter-*` | narrows a result filter or a group; see [Filters](#filters) || Run name | positional `NAME` of `show` and `wait` | a run by its `--run-name` label |

A command, not a task, is the unit of queue edits: an array command is
selected as a whole by its job ID or name. Commands that act on results or
output (`show`, `copy`, `run`, `retry`) may also select one task.
`resolve.JobInQueue` matches a command's own ID or name before its tasks'.

Without `--project-name` (or its environment and config defaults), a job ID
or job name searches every project of the base directory in every command:
the places each command reads, below, in each project. A selector that
matches in more than one place fails with `resolve.AmbiguousError`, which
lists each candidate's project, run or queue, and job. A group selector
(`--stage`, `--matrix`, `--all`) still needs a single project.

## What each command reads

| Command | Without `--run-id` | With `--run-id` |
| --- | --- | --- |
| `show` | The active run, then an interrupted run, then a non-empty queue, then the latest run. Options that only apply to runs (`--failed`, the `--filter-*` conditions on a result or execution such as `--filter-exit-code` and `--filter-host`, `--logs`, `--failed-logs`, `--follow`, `--report`) skip the queue, and `--queue` rejects them. A job selector without a project searches every project the same way and fails when it is ambiguous. | That run. |
| `export` | A non-empty queue, else the latest run, named on stderr. An active or interrupted run is refused, pointing to `wait` or `unlock`. | That run, refused the same way when it is active or interrupted. |
| `copy` | Source: the run named by attempt IDs, else the latest run that holds every `--job-id` (searching every project without `-p`), else the project's latest run. Destination: the current queue. | Source: that run (also as positional `RUN_ID`). |
| `run`, `retry` | The current queue. A job selector, result filter, or group first restores the queue from the reference run when it is empty. A job selector looks for the job in a non-empty queue first, as `show` does, and otherwise in the latest run, which then replaces the queue (after confirmation). The reference run is found like `copy`'s source. | The queue is replaced by that run's snapshot (after confirmation), which is also the reference. |
| `change`, `remove` | The current queue; a job ID or name without a project is looked for in every project's queue. An empty queue is not restored; the command fails and points to `copy` and `--run-id`. | That run's snapshot replaces the queue first. |
| `cancel`, `suspend`, `resume` | The project's active run; a job ID without a project is looked for in the active run of every project, and all job IDs must be in one. | No `--run-id`: a bare run ID or an attempt ID names the run, which must be the project's active run. |

**SEL-2** Each command reads the run or queue that its row names, with and
without `--run-id`.

## Job selectors by command

Cells describe the intended behavior; "error" means the command rejects the
form with a message that names the problem. Entries marked † differ today;
see [Known deviations](#known-deviations).

| Form | `show` | `copy` | `run`, `retry` | `change` | `remove` |
| --- | --- | --- | --- | --- | --- |
| Job ID | that job | that job | only that job executes; in `retry`, instead of failed and unfinished jobs | that command | that command; repeatable, or positional |
| Array command ID | a table of the array's tasks | the whole array | the whole array | the whole array | the whole array |
| Array task ID | that task | that task, narrowing the array | that task; the array's other tasks carry forward | error with the array job's ID | error with the array job's ID |
| Attempt ID | that attempt (also positional) | that attempt, narrowing an array to its task | copies that attempt, then runs it | error naming the attempt's job ID | error naming the attempt's job ID |
| Job name | that job; ambiguous across projects without `-p` | same as `show` | same as `show` | that command | that command |
| Array job name | a table of the array's tasks | the whole array | the whole array | the whole array | the whole array |
| Array task name | that task | that task, narrowing the array | that task; the array's other tasks carry forward | error with the array job's ID | error with the array job's ID |
| `--stage`, `--matrix` | filter the job table | narrow the selection | narrow the selection | every matching command | every matching command |
| `--all` | – | default without a selector | default without a selector | every command | every command |
| Result filter | `--failed` only | select by result | select by result | – | – |
| Positional | an attempt ID, then a registered run ID; otherwise a saved run name, job ID, or job name, where more than one match fails | run ID | – | – | job IDs |

- **SEL-3** `show` resolves each form as its column says.
- **SEL-4** `copy` resolves each form as its column says.
- **SEL-5** `run` and `retry` resolve each form as their column says.
- **SEL-6** `change` resolves each form as its column says.
- **SEL-7** `remove` resolves each form as its column says.

Selector combinations:

Options that choose jobs are of three kinds, and the kind decides how they
combine:

| Kind | Options | Combination |
| --- | --- | --- |
| Direct | `--job-id` (job, task, or attempt ID), `--job-name`, positional job IDs | the named jobs, and nothing else |
| Result filter | `--failed`, `--unfinished`, `--success`, `--filter-result` | with each other: any of them (OR) |
| Scope | `--stage`, `--matrix`, `--filter-stage`, `--filter-matrix` | narrows a result filter (AND); alone, every job in it |
| Filter | `--filter-not-stage`, `--filter-not-matrix` | narrows a result filter and a scope (AND); alone, every job it keeps |

This follows the usual command-line convention, as in `git log`, that repeated
options of one kind widen the match and options of different kinds narrow it
(`--grep A --grep B` against `--author X --grep Y`). Direct selectors are the
exception: naming a job already says what to execute, so a result filter or a
scope on top of it could only drop the job again, and combining them is an
error rather than an intersection.

`retry` is therefore not an alias of `run --failed --unfinished`. As an alias,
`retry -j ID` would expand to a direct selector with a result filter and be
rejected. Instead `--failed --unfinished` is `retry`'s default, used only when
neither a result filter nor a direct selector is given: `retry -j ID` runs only
that job, exactly as `run -j ID` does, and `retry --success` runs successful
jobs. Implemented by `runJobs` in
[cmd/rotari/run_command.go](../cmd/rotari/run_command.go); covered by the
`retry` rows of `TestSelectorTable` in
[conformance/06-selectors/selector_test.go](../conformance/06-selectors/selector_test.go).

An edited job keeps its recorded result; a job marked
`change --status unfinished` has no result until it runs again (see
[02-run-lifecycle-and-execution.md](02-run-lifecycle-and-execution.md)), so
`retry` runs failed jobs and jobs marked unfinished together without naming
them.

**SEL-8** Selectors combine by kind as described above, and these exclusions
hold:

- `--job-id` and `--job-name` exclude each other in every command.
- `--all` means "every job" (`change`, `remove`) or "every run" (`delete`)
  and nothing else; the option that widens a listing is named for what it adds:
  `jobs --all-basedirs`. Every `--all` is
  command-line only.
- `--stage`, `--matrix`, and `--all` exclude each other and the job
  selectors in `change` and `remove`; `--stage` and `--matrix` exclude job
  selectors in `copy`, `run`, and `retry`, and combine with a result filter.
- Job selectors exclude result filters in `copy`, `run`, and `retry`, and
  `run.PlanRerun` and `queueedit.Copy` reject the combination from any caller.
  The Web UI's copy endpoint rejects `job_id` with a `selection` the same way.
- `--filter-*` options exclude job selectors in `copy`, `run`, and `retry`,
  and `--job-id` in `show`, like `--stage` and `--matrix`; `run.PlanRerun` and
  `queueedit.Copy` reject a job filter with job IDs.
- An attempt ID fixes the run; a `--run-id` naming another run is an error.
- `change` and `remove` edit commands, so a new command or `--set-job-name`
  needs a single job.

### Filters

Every option that filters jobs is named `--filter-*`, so filters are told
apart from other options at a glance and never collide with a job setting of
the same name. `show`, `copy`, `run`, and `retry` list them under their own
heading in `--help`. `change` and `remove` take only the definition filters
(`--filter-command`, `--filter-stage`, `--filter-matrix`, and their
`--filter-not-*` forms); `--filter-changed` and `--filter-new` are taken by
`show`, `run`, and `retry` only. `show --json` keeps the jobs that its job
table keeps for `--failed`, in the run's summary results and in a job or
array view's jobs.

For run-wide log output, `show --logs --failed` and
`show --logs --filter-result failed` select the same failed jobs as
`show --failed-logs`, including carried output. Other result selections are
rejected with log output. The CLI forwards the parsed failed selection to the
existing log selector in [cmd/rotari/show.go](../cmd/rotari/show.go); the
external check is
[conformance/03-interfaces/log_selection_test.go](../conformance/03-interfaces/log_selection_test.go).

**SEL-11** The `--filter-*` options select jobs as follows:

- `--filter-result RESULT` (`failed`, `unfinished`, or `success`) is the
  long form of `--failed`, `--unfinished`, and `--success`; repeated, and
  together with them, the results combine with OR.
- `--filter-exit-code N` matches jobs whose resolved exit code equals N; repeated values combine with OR.
- `--filter-failure-kind KIND` matches jobs whose result falls into one of the failure kinds (`timeout`, `cancelled`, `blocked`, `oom`, `signal`, `error`); repeated values combine with OR.
- `--filter-diagnosis VALUE` recomputes the latest attempt's diagnosis with
  the current rules and matches failed jobs. A rule slug ID is matched first;
  otherwise VALUE is a case-insensitive substring of the rule name. Repeated
  values combine with OR. Saved diagnoses and unavailable/no-match outcomes do
  not match.
- `--filter-changed` matches queued jobs whose fingerprint differs from the
  matching job in the reference run; `--filter-new` matches queued jobs with
  no matching job in that run, and every job when there is no reference run.
  The reference run is `--run-id` or else the project's last run, resolved
  before the new run is recorded; matching follows `--match-by` (`show` uses
  `id-and-fingerprint`). Given together, a job must satisfy both. `show`
  applies them to the queue view only and rejects them for a run view.
- `--filter-host PATTERN` matches jobs whose latest attempt ran on a host
  matching the `path.Match` glob; repeated patterns combine with OR.
- `--filter-started-after`, `--filter-started-before`,
  `--filter-finished-after`, and `--filter-finished-before` compare the latest
  attempt's timestamps. `after` is inclusive and `before` is exclusive.
- For a job whose result was carried into the run, the host, time, and
  diagnosis conditions read the attempt that produced the result, in the run
  it was carried from.
- `--filter-longer-than` and `--filter-shorter-than` compare execution
  duration. The former is inclusive and the latter is exclusive. A running
  job uses the current time as its end; a missing timestamp does not match.
- `--filter-stage` and `--filter-matrix` are the long forms of `--stage` and
  `--matrix`, which name one stage or matrix: given together with its short
  form, the value must be the same.
- `--filter-not-stage NAME` and `--filter-not-matrix NAME` exclude the jobs
  of a stage or of a matrix, by base job name; repeated, a job matching any
  value is excluded. A job without a stage or matrix is not excluded.
- A filter narrows a result filter and a scope. Alone, it keeps every job it
  does not exclude: `run` executes them whatever their result, and `retry`
  narrows its default failed and unfinished jobs.
- An array job decided as a whole (`copy`, and `run`/`retry` with
  `--partial-array=false`) matches a result filter by its aggregate result,
  finished when every task is and failed when any task is, and matches the
  other `--filter-*` conditions when any one task does.

The conditions are `jobfilter.Filter` in
[internal/jobfilter/filter.go](../internal/jobfilter/filter.go). Its
`Selects` decides each job against the result selection and the per-job
conditions for `run.PlanRerun` (whole commands and array tasks),
`queueedit.Copy`, and `show`, and `SelectsArray` decides an array job as a
whole from its tasks; `MatchesCommand` applies the definition
conditions. `jobstatus.FilterJob` in
[internal/jobstatus/facts.go](../internal/jobstatus/facts.go) reads the
hosts, times, and log each job is judged by. The options are
`cliJobFilterOptions` in
[cmd/rotari/job_filter_flags.go](../cmd/rotari/job_filter_flags.go).
Covered by the filter rows of `TestSelectorTable`.

### Job control

`cancel`, `suspend`, and `resume` act on the running jobs of a project's
active run. They take job IDs and attempt IDs, positionally or as repeated
`--job-id`, job names as repeated `--job-name`, or filters, and no result
filter. Resolution is
`resolve.JobSelection`; the running-run check and the signalling are
`jobcontrol.Controller` in
[internal/jobcontrol/jobcontrol.go](../internal/jobcontrol/jobcontrol.go),
shared with the Web UI's cancel, suspend, and resume endpoints. Those
endpoints require the `run_id` of the run the page shows, so a page left
open on a finished run cannot act on the same job ID in the active run
(`TestWebJobControlRejectsStaleRunID` in
[internal/webui/webui_test.go](../internal/webui/webui_test.go)).

| Form | `cancel` | `suspend`, `resume` |
| --- | --- | --- |
| None | the whole run | every running job |
| Job ID | that job, submitted or not | that job, which must be running |
| Array command ID | its unfinished tasks | its running tasks |
| Array task ID | that task | that task |
| Attempt ID | its job; it must be the job's latest attempt, running | same |
| Bare run ID | only names the run: none or the other IDs apply | same |
| Run ID not active | error naming the project's active run, if any | same |
| Job ID in no active run | error, when no project is given | same |
| `latest` | a job ID like any other; not a run | same |

**SEL-9** `cancel`, `suspend`, and `resume` resolve each form as the table
says.

**SEL-12** `--job-name` and the filters choose unfinished jobs of the
active run, array tasks one by one, through `jobcontrol.Controller.Select`:

- `--job-name NAME` selects the jobs, or array command, with that name;
  repeated, names combine with OR. A name no job of the run has is an error.
- The filters are `--stage`, `--matrix`, `--filter-command`, `--filter-host`,
  `--filter-started-after`, `--filter-started-before`,
  `--filter-longer-than`, `--filter-shorter-than`, the `--filter-not-*` forms
  of stage and matrix, and `--filter-state running|pending` (repeated, OR).
  A job is running once its executor owns it and pending before; a running
  job's duration ends now. `suspend` and `resume` accept only `running`.
- Job IDs, `--job-name`, and the filters exclude one another, and all exclude
  `cancel --wait`; a bare run ID may still name the run.
- Without `--filter-state`, `cancel` selects running and pending jobs and
  `suspend` and `resume` running ones.
- A selection matching no job is an error, never a whole-run cancel.
- A filtered selection lists its jobs on a terminal and asks before acting;
  `--yes` skips the question and is required without a terminal. The jobs
  listed are the ones acted on; the filters are not evaluated again.

Covered by `TestSelectJobsFiltersUnfinishedJobs` in
[internal/jobcontrol/jobcontrol_test.go](../internal/jobcontrol/jobcontrol_test.go)
and the job-control tests in
[cmd/rotari/job_control_test.go](../cmd/rotari/job_control_test.go).

`cancel --wait` takes no job selection. Unlike `show` and `wait`, these
commands take no run name or positional project: they act on running jobs, so
they name them only by IDs and `--project-name`, and a free-form run name could
not be told apart from a job ID in the same list. Covered by `TestJobControlSelectors`
in [conformance/06-selectors/job_control_test.go](../conformance/06-selectors/job_control_test.go),
against the fixture with run `live` of project `sweep` active, and by the
`JobSelection` tests in
[internal/resolve/resolve_test.go](../internal/resolve/resolve_test.go).

## Positional arguments

General rules:

- Options may come before, between, or after positional arguments
  (`copy RUN_ID --overwrite`), except in `add` and `change`. `--` ends the
  options: every later argument is positional, which is how a positional that
  starts with `-` is passed. Implemented by `cliParse` in
  [cmd/rotari/cli_spec.go](../cmd/rotari/cli_spec.go).
- In `add` and `change`, options must come before the job command: parsing
  stops at the first positional argument, and every later argument belongs to
  the job command even when it looks like a rotari option, so
  `add python train.py --timeout 30` passes `--timeout 30` to the script.
- A positional argument that stands for an option cannot be combined with
  that option: supplying both is a usage error, not a precedence rule.
- A command without positional arguments rejects any with a usage error.
- A positional argument means the same thing whatever options are given: an
  option never turns a project into a run ID. Where a positional may name a
  project or something else, the project is tried first, as in `show` and
  `wait`.
- Project, run, and job values are path elements, never paths (see
  [01-resolution-and-config.md](01-resolution-and-config.md)); only `FILE` of
  `export` and `import` and `MASTERDIR` of `gc` are filesystem paths.

| Command | Positional | Meaning | Excludes |
| --- | --- | --- | --- |
| `add`, `change` | `<command ...>` | the job command: every argument from the first positional on | – |
| `check`, `reset` | `[PROJECT]` | the project | `--project-name` |
| `jobs` | `[PROJECT]` | the project to list; overrides environment and config defaults | `--project-name` |
| `unlock` | `[PROJECT]` | the project; the run ID to verify is always `--run-id` | `--project-name` |
| `show` | `[SELECTOR]` | an attempt ID, then a registered run ID or `latest`, then a project (unless `--project-name` is given); otherwise a run name (active or saved), job ID, or job name, where more than one match is ambiguous | run, job, queue, and list options; a run name, job ID, or job name also excludes log, follow, JSON, and report options, which an attempt ID, run ID, `latest`, or project takes like its option |
| `wait` | `[SELECTOR ...]` | each: `latest`, then a project, then a run name, then a registered run ID; a project or run name means its active run, or else its latest run | – (added to `--run-id`) |
| `cancel`, `suspend`, `resume` | `[ID ...]` | job IDs, attempt IDs, and a bare run ID that names the run, which must be active (see [Job control](#job-control)) | `--job-id` |
| `remove` | `[JOB_ID ...]` | job IDs | `--job-id` |
| `copy`, `run`, `retry` | `[RUN_ID]` | the run to copy or rerun from | `--run-id` |
| `delete` | `[RUN_ID]` | the run to delete; without one, `--all` must be given to delete every run | `--run-id`, `--all` |
| `lineage` | `[RUN_ID ...]` | none: the project's runs oldest first; one: that run's summary; two: compare the runs; three or more: show a job-by-run result grid | – |
| `export` | `[TARGET] [FILE]` | `TARGET` is a run when it has a run ID's shape or is `latest`, otherwise a project (see What each command reads); `FILE` is the output | a project `TARGET` excludes `--project-name`; `FILE` excludes `--output` |
| `import` | `FILE [PROJECT]` | the manifest, and the destination project; a run-exported manifest must come from that project | `PROJECT` excludes `--project-name` |
| `diagnose` | `[JOB_ID]` | a job ID or attempt ID; `--job-name` names the job instead | `--job-id`, `--job-name` |
| `gc` | `[MASTERDIR]` | the master directory | `--masterdir` |
| others | none | – | – |

**SEL-10** Each command takes the positional arguments of its row, with that
meaning, and rejects them together with the options they exclude; the
general rules above hold.

`TestPositionalArguments` in
[conformance/06-selectors/positional_test.go](../conformance/06-selectors/positional_test.go) covers
each row against the fixture, except `cancel`, `suspend`, and `resume`,
which `TestJobControlSelectors` covers against a running run (see
[Job control](#job-control)).

## Known deviations

None at present. A deviation found later is listed here and in
marked with a † in the tables, and its test
row skips through `knownDeviation` until it is fixed.

## Fixture

`newSelectorFixture` in
[conformance/06-selectors/selector_fixture_test.go](../conformance/06-selectors/selector_fixture_test.go)
builds, with the binary, a base directory with project `sweep` (a plain job,
a matrix with a retried failure, an array with a failed task, and an unnamed
job, each in its own stage; runs `first` and `second`), project `other` (a
job that shares the name `prep`; a run also named `first`), and a second base
directory reachable only through the run registry. Symbolic keys map to the
generated run, job, and attempt IDs. The environment has no location
variables, so commands find a base directory only through `-b` or the run
registry. `TestSelectorFixtureLayout` checks the layout.

The job control rows add run `live` of project `sweep`, run in the
background, with an array job `hold` of two tasks and a job `idle`, each
sleeping (`startJobControlRun` in
[conformance/06-selectors/job_control_test.go](../conformance/06-selectors/job_control_test.go)). The
positional rows that need an active run use a run `live` with one sleeping
job, and those that need an interrupted one kill its supervisor and job
(`setUp` in [conformance/06-selectors/positional_test.go](../conformance/06-selectors/positional_test.go)).
