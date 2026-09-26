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
| Run ID | `--run-id/-r ID`, `latest` | a saved run; a registered ID also names its base directory and project; `latest` is the project's latest run, and no name may be `latest` |
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
| `show` | The active run, then an interrupted run, then a non-empty queue, then the latest run. Options that only apply to runs (`--failed`, `--logs`, `--failed-logs`, `--follow`, `--report`) skip the queue. A job selector without a project searches every project the same way and fails when it is ambiguous. | That run. |
| `export` | A non-empty queue, else the latest run, named on stderr. An active or interrupted run is refused, pointing to `wait` or `unlock`. | That run, refused the same way when it is active or interrupted. |
| `copy` | Source: the run named by attempt IDs, else the latest run that holds every `--job-id` (searching every project without `-p`), else the project's latest run. Destination: the current queue. | Source: that run (also as positional `RUN_ID`). |
| `run`, `retry` | The current queue. A job selector, result filter, or group first restores the queue from the reference run when it is empty. A job selector looks for the job in a non-empty queue first, as `show` does, and otherwise in the latest run, which then replaces the queue (after confirmation). The reference run is found like `copy`'s source. | The queue is replaced by that run's snapshot (after confirmation), which is also the reference. |
| `change`, `remove` | The current queue; a job ID or name without a project is looked for in every project's queue. An empty queue is not restored; the command fails and points to `copy` and `--run-id`. | That run's snapshot replaces the queue first. |

## Job selectors by command

Cells describe the intended behavior; "error" means the command rejects the
form with a message that names the problem. Entries marked † differ today;
see [Known deviations](#known-deviations).

| Form | `show` | `copy` | `run`, `retry` | `change` | `remove` |
| --- | --- | --- | --- | --- | --- |
| Job ID | that job | that job; with a result filter, in addition to the matching jobs | only that job executes; with a result filter (always in `retry`), in addition to the matching jobs | that command | that command; repeatable, or positional |
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

Selector combinations:

- `--job-id` and `--job-name` exclude each other in every command.
- `--all` means "every job" (`change`, `remove`) or "every run" (`delete`)
  and nothing else; the options that widen a listing are named for what they
  add: `diff --unchanged` and `jobs --all-basedirs`. Every `--all` is
  command-line only.
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
| `show` | `[SELECTOR]` | an attempt ID, then a registered run ID or `latest`, then a project (unless `--project-name` is given); otherwise a run name (active or saved), job ID, or job name, where more than one match is ambiguous | run, job, queue, log, JSON, and report options |
| `wait` | `[SELECTOR ...]` | each: `latest`, then a project, then a run name, then a registered run ID; a project or run name means its active run, or else its latest run | – (added to `--run-id`) |
| `cancel`, `suspend`, `resume` | `[ID ...]` | job IDs, attempt IDs, and a bare run ID that only locates the run | `--job-id` |
| `remove` | `[JOB_ID ...]` | job IDs | `--job-id` |
| `copy` | `[RUN_ID]` | the run | `--run-id` |
| `delete` | `[RUN_ID]` | the run to delete; without one, `--all` must be given to delete every run | `--run-id`, `--all` |
| `diff` | `[[RUN_A] RUN_B]` | none: the latest run against the run before it; one: that run against the run before it; two: the runs, which must belong to one project | – |
| `export` | `[TARGET] [FILE]` | `TARGET` is a run when it has a run ID's shape or is `latest`, otherwise a project (see What each command reads); `FILE` is the output | a project `TARGET` excludes `--project-name`; `FILE` excludes `--output` |
| `import` | `FILE [PROJECT]` | the manifest, and the destination project; a run-exported manifest must come from that project | `PROJECT` excludes `--project-name` |
| `diagnose` | `JOB_ID` | a job ID or attempt ID | `--job-id` |
| `gc` | `[MASTERDIR]` | the master directory | `--masterdir` |
| others | none | – | – |

`TestPositionalArguments` in
[cmd/rotari/positional_test.go](../../cmd/rotari/positional_test.go) covers
each row against the fixture, except `cancel`, `suspend`, and `resume`,
whose selector resolution is covered by `resolve.JobSelection` tests in
[internal/resolve/resolve_test.go](../../internal/resolve/resolve_test.go).

## Known deviations

None at present. A deviation found later is listed here and in
[ISSUES.md](../../ISSUES.md), marked with a † in the tables, and its test
row carries a `known` mark until it is fixed.

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
