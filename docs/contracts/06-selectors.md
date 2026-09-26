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
- [cmd/rotari/selector_cases_test.go](../../cmd/rotari/selector_cases_test.go):
  the tables below as test cases, run by `TestSelectorTable` in
  [selector_test.go](../../cmd/rotari/selector_test.go) against the shared
  fixture in [selector_fixture_test.go](../../cmd/rotari/selector_fixture_test.go)
  (see [Fixture](#fixture)). A row marked with a known deviation must fail
  until the deviation is fixed.

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
| `change`, `remove` | The current queue. An empty queue is not restored; the command fails and points to `copy` and `--run-id`. | That run's snapshot replaces the queue first. |

## Job selectors by command

Cells describe the intended behavior; "error" means the command rejects the
form with a message that names the problem. Entries marked † differ today;
see [Known deviations](#known-deviations).

| Form | `show` | `copy` | `run`, `retry` | `change` | `remove` |
| --- | --- | --- | --- | --- | --- |
| Job ID | that job | that job | only that job executes (`retry`: in addition to failed and unfinished jobs †) | that command | that command; repeatable, or positional |
| Array command ID | the array's tasks † | the whole array † | the whole array † | the whole array | the whole array |
| Array task ID | that task | that task, narrowing the array † | that task † | error with the array job's ID | error with the array job's ID |
| Attempt ID | that attempt (also positional) | that attempt, narrowing an array to its task | copies that attempt, then runs it | error † | error † |
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

## Positional arguments

General rules:

- Options may come before, between, or after positional arguments
  (`copy RUN_ID --overwrite`), except in `add` and `change`. `--` ends the
  options: every later argument is positional, which is how a positional that
  starts with `-` is passed. Implemented by `cliParse` in
  [cmd/rotari/cli_spec.go](../../cmd/rotari/cli_spec.go).
- In `add` and `change`, options must come before the job command: parsing
  stops at the first positional argument, and every later argument belongs to
  the job command even when it looks like a rotari option, so
  `add python train.py --timeout 30` passes `--timeout 30` to the script.
- A positional argument that stands for an option cannot be combined with
  that option: supplying both is a usage error, not a precedence rule.
- A command without positional arguments rejects any with a usage error.
- Project, run, and job values are path elements, never paths (see
  [01-resolution-and-config.md](01-resolution-and-config.md)); only `FILE` of
  `export` and `import` and `MASTERDIR` of `gc` are filesystem paths.

| Command | Positional | Meaning | Excludes |
| --- | --- | --- | --- |
| `add`, `change` | `<command ...>` | the job command: every argument from the first positional on | – |
| `check`, `reset` | `[PROJECT]` | the project | `--project-name` |
| `jobs` | `[PROJECT]` | the project to list; overrides environment and config defaults | `--project-name` |
| `unlock` | `[PROJECT]` | the project; with `--project-name`, the run ID to verify instead | `--project-name` together with `--run-id` |
| `show` | `[SELECTOR]` | an attempt ID, then a registered run ID; otherwise a saved run name, job ID, or job name, where more than one match is ambiguous. Not a project name. | run, job, queue, log, JSON, and report options |
| `wait` | `[SELECTOR ...]` | each: a project (its active run), then an active run name, then a registered run ID | – (added to `--run-id`) |
| `cancel`, `suspend`, `resume` | `[ID ...]` | job IDs, attempt IDs, and a bare run ID that only locates the run | `--job-id` |
| `remove` | `[JOB_ID ...]` | job IDs | `--job-id` |
| `copy`, `delete` | `[RUN_ID]` | the run | `--run-id` |
| `diff` | `[[RUN_A] RUN_B]` | none: the latest run against the run before it; one: that run against the run before it; two: the runs, of one project | – |
| `export` | `[TARGET] [FILE]` | `TARGET` is a run ID when it has a run ID's shape or `--project-name` is given, otherwise a project, whose queue is exported; `FILE` is the output | `FILE` excludes `--output` |
| `import` | `FILE [PROJECT]` | the manifest, and the destination project; a run-exported manifest must come from that project | `PROJECT` excludes `--project-name` |
| `diagnose` | `JOB_ID` | a job ID or attempt ID | `--job-id` |
| `gc` | `[MASTERDIR]` | the master directory | `--masterdir` |
| `run` | `config` | an undocumented alias of `rotari config` | – |
| others | none | – | – |

`TestPositionalArguments` in
[cmd/rotari/positional_test.go](../../cmd/rotari/positional_test.go) covers
each row against the fixture, except `cancel`, `suspend`, and `resume`,
whose selector resolution is covered by `resolve.JobSelection` tests in
[internal/resolve/resolve_test.go](../../internal/resolve/resolve_test.go).

## Known deviations

Each is also recorded in [ISSUES.md](../../ISSUES.md) until it is resolved.

- `show`, `copy`, `run`, and `retry` look jobs up among expanded tasks
  (`resolve.JobInQueue` and `resolve.JobInRun`), so an array command's own ID
  or name is "not found", and a task found this way is then not found by
  `copy` and the rerun plan, which select commands.
- `retry --job-id` (and `run --failed --job-id`) ignores the job IDs: with a
  result filter, `planByOrigin` in
  [internal/run/rerun.go](../../internal/run/rerun.go) drops them.
- `change` and `remove` report an attempt ID as "job not found" instead of
  saying that attempt IDs are not accepted.

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
