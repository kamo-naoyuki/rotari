# Job and project selectors

This note tabulates how commands turn their location and job selectors into
the project, run, and jobs they act on. The resolution rules themselves
(base directory, project, run registry, `latest`) are in
[01-resolution-and-config.md](01-resolution-and-config.md); this note is the
per-command view, and the reference for selector tests.

Representative implementation and tests:

- [internal/resolve/resolve.go](../../internal/resolve/resolve.go) and
  [its tests](../../internal/resolve/resolve_test.go): locations, run IDs,
  attempt IDs, and job lookup in a queue or run.
- [internal/model/command_selector.go](../../internal/model/command_selector.go)
  and [its tests](../../internal/model/command_selector_test.go): selecting
  queue commands by job ID, name, stage, matrix, or all.
- [cmd/rotari/selector_fixture_test.go](../../cmd/rotari/selector_fixture_test.go):
  the shared fixture for command-level selector tests (see [Fixture](#fixture)).

## Selector forms

| Form | Example | Names |
| --- | --- | --- |
| Base directory | `--basedir/-b DIR`, `ROTARI_BASEDIR` | the state directory |
| Project | `--project-name/-p NAME`, `ROTARI_PROJECT_NAME`, the only project | a project |
| Run ID | `--run-id/-r ID`, `latest` | a saved run; a registered ID also names its base directory and project |
| Job ID | `--job-id/-j ID` | a queued command |
| Array task ID | `ID-2` | one task of an array command |
| Attempt ID | `att_<run>-<job>-<n>` | one execution attempt; also names its run |
| Job name | `--job-name NAME` | a named command |
| Array task name | `NAME[2]` | one task of a named array command |
| Group | `--stage STAGE`, `--matrix NAME`, `--all` | every command of a stage, of a matrix (by base job name), or of the queue |
| Result filter | `--failed`, `--unfinished`, `--success` | jobs by their result in the reference run |
| Run name | positional `NAME` of `show` and `wait` | a run by its `--run-name` label |

A command, not a task, is the unit of queue edits: an array command is
selected as a whole by its job ID or name. Commands that act on results or
output (`show`, `copy`, `run`, `retry`) may also select one task.

## What each command reads

| Command | Without `--run-id` | With `--run-id` |
| --- | --- | --- |
| `show` | The active run, then an interrupted run, then a non-empty queue, then the latest run. A job selector without a project searches every project the same way and fails when it is ambiguous. | That run. |
| `copy` | Source: the run named by attempt IDs, else the latest run that holds every `--job-id` (searching every project without `-p`), else the project's latest run. Destination: the current queue. | Source: that run (also as positional `RUN_ID`). |
| `run`, `retry` | The current queue. A job selector, result filter, or group first restores the queue from the reference run when it is empty; a job selector always does. The reference run is found like `copy`'s source. | The queue is replaced by that run's snapshot (after confirmation), which is also the reference. |
| `change` | The current queue. An empty queue is not restored. | That run's snapshot replaces the queue first. |
| `remove` | The current queue, or the latest run's snapshot when the queue is empty. | That run's snapshot replaces the queue first. |

## Job selectors by command

Cells describe the intended behavior; "error" means the command rejects the
form with a message that names the problem. Entries marked † differ today;
see [Known deviations](#known-deviations).

| Form | `show` | `copy` | `run`, `retry` | `change` | `remove` |
| --- | --- | --- | --- | --- | --- |
| Job ID | that job | that job | only that job executes (`retry`: in addition to failed and unfinished jobs †) | that command | that command; repeatable, or positional |
| Array command ID | the array's tasks † | the whole array † | the whole array † | the whole array | the whole array |
| Array task ID | that task | that task, narrowing the array † | that task † | error with the array job's ID | error with the array job's ID |
| Attempt ID | that attempt (also positional) | that attempt, narrowing an array to its task | copies that attempt, then runs it | error | error † |
| Job name | that job; ambiguous across projects without `-p` | same as `show` | same as `show` | that command | that command |
| Array job name | the array's tasks † | the whole array † | the whole array † | the whole array | the whole array |
| Array task name | that task | that task † | that task † | error with the array job's ID | error with the array job's ID |
| `--stage`, `--matrix` | filter the job table | narrow the selection | narrow the selection | every matching command | every matching command |
| `--all` | – | default without a selector | default without a selector | every command | every command |
| Result filter | `--failed` only | select by result | select by result | – | – |
| Positional | an attempt ID, then a registered run ID; otherwise a saved run name, job ID, or job name, where more than one match fails | run ID | – | – | job IDs |

Selector combinations:

- `--job-id` and `--job-name` exclude each other in every command.
- `--stage`, `--matrix`, and `--all` exclude each other and the job
  selectors in `change` and `remove`; `--stage` and `--matrix` exclude job
  selectors in `copy`, `run`, and `retry`, and combine with a result filter.
- An attempt ID fixes the run; a `--run-id` naming another run is an error.
- `change` and `remove` edit commands, so a new command or `--set-job-name`
  needs a single job.

## Known deviations

Each is also recorded in [ISSUES.md](../../ISSUES.md) until it is resolved.

- `show`, `copy`, `run`, and `retry` look jobs up among expanded tasks
  (`resolve.JobInQueue` and `resolve.JobInRun`), so an array command's own ID
  or name is "not found", and a task found this way is then not found by
  `copy` and the rerun plan, which select commands.
- `retry --job-id` (and `run --failed --job-id`) ignores the job IDs: with a
  result filter, `planByOrigin` in
  [internal/run/rerun.go](../../internal/run/rerun.go) drops them.
- `remove --job-id ATTEMPT_ID` reports "job not found" instead of saying that
  attempt IDs are not accepted.

## Fixture

`newSelectorFixture` in
[cmd/rotari/selector_fixture_test.go](../../cmd/rotari/selector_fixture_test.go)
builds, through the real `add` and run paths, a base directory with project
`sweep` (a plain job, a matrix with a retried failure, an array with a failed
task, and an unnamed job, each in its own stage; runs `first` and `second`),
project `other` (a job that shares the name `prep`; a run also named
`first`), and a second base directory reachable only through the run
registry. Symbolic keys map to the generated run, job, and attempt IDs.
Location variables and user config are cleared, so results do not depend on
the caller's environment. `TestSelectorFixtureLayout` checks the layout.
