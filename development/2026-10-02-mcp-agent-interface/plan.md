# Plan: Agent-Facing MCP Interface

**Created:** 2026-10-02
**Work history:** [work-log.md](work-log.md)

## Purpose

Make rotari efficient for agents to diagnose, fix, and rerun jobs, and eventually to request other rotari operations, without being given filesystem paths or replaying human-oriented screen flows.

The [agent trial](agent-trial-2026-10-02.md) showed that an agent with a shell can already do this through the CLI: run IDs resolve without paths, `show -r RUN_ID --json` carries per-job diagnoses, and `lineage RUN1 RUN2` compares runs by job name. The cost is volume, not capability: deciding what to fix in a 300-task run took about 100 KB of output when the decision needs under 1 KB. The plan therefore puts agent-specific *information* first, in shared packages that every interface uses, and treats MCP as a thin delivery channel for clients without a shell.

This is a design/implementation exploration. API names, schemas, and identifiers are not a public contract until a milestone below says so.

## Scope and non-goals

In scope, in milestone order (see [Milestones](#milestones)):

- compact, decision-ready information: failures grouped by cause, relevant log excerpts, cause-aware run comparison, incremental progress, and cross-basedir discovery with results;
- delivering it through the CLI (`--json` and human output), the Web UI where useful, and read-only MCP tools;
- later, read-only operations (`check`, `export`) and then explicitly named state-changing and lifecycle operations behind a preview/apply boundary.

Non-goals:

- a second implementation of `cmd/rotari` behavior inside an agent or MCP adapter;
- a separate terminal agent command; terminal agents use `rotari` itself;
- a generic object query language, a generic command interpreter, an untyped `operation: "<CLI command>"` field, or arbitrary shell commands;
- exposing every CLI flag through MCP; a tool exposes a coherent use case and the selectors it needs;
- a visual UI or a dedicated VS Code extension.

## Principles

1. **Information before interfaces.** The trial's gaps are missing summaries, not missing entry points. Build each summary once in a shared package; the CLI, Web UI, and MCP present it.
2. **The CLI is the terminal agent interface.** Agents with a shell use `rotari` commands and their `--json` output. Improvements for agents land there, with stable JSON fields, so humans and scripts benefit too.
3. **MCP is a thin adapter.** `internal/mcp` maps typed tool inputs to the same shared functions the CLI calls and encodes their results. It holds no selection, status, or grouping rules of its own.
4. **One rule, one owner.** Each behavior uses the package that already owns it (see [Shared behavior map](#shared-behavior-map)). When a capability exists only inside a `cmd/rotari` handler, extract it to a lower-level package first; do not copy the handler.
5. **Read and change are separate tools.** A state change is a separately named operation with typed arguments, never a side effect of a read. MCP clients ask the user per tool, so separating read tools from changing and destructive tools lets users always allow reads and confirm changes.
6. **Never guess, never ignore.** Ambiguous matches are returned as candidates. Unsupported selectors or options fail with an error rather than being accepted and ignored.
7. **Bounded and safe by default.** Output states its scope, counts, and truncation. Secrets, environment values, executor options, and sensitive paths are not returned by default; commands and logs are treated as potentially sensitive.

## Decisions

Record any reversal here with its reason.

- **No `rotari-agent`.** The prototype's `rotari-agent` printed the same report as `rotari show --report`. It is removed rather than extended (M0).
- **MCP configuration.** One MCP server process serves one master directory, resolved at startup like the CLI's. A run is reachable only through that master directory's run registry (`resolve.RegisteredRun`), with no fallback to a default or working-directory basedir. Discovery tools (M4) will list basedirs from the same master directory.
- **Basedir reference in MCP.** A basedir is addressed by its registry record key (the 32-hex prefix of the SHA-256 of its absolute path, as `basedirregistry` already uses), which is stable while the path is unchanged. MCP results do not return the absolute path by default. Run and attempt IDs remain the primary handles because they already resolve their basedir and project.
- **MCP mechanism.** Tools only at first. Resources and Prompts may be added later without changing the shared functions.
- **Task-shaped tools.** MCP exposes a small number of tools named after use cases (for example, find recent failures, summarize a run's failures, show a job's evidence, compare runs), not a generic query tool. Results do not advertise `available_operations`; tool schemas describe what exists.
- **Structured output.** Results are structured records. Human-formatted text, such as `report.Build` output, may be one field, not the whole result.
- **Dependents rerun with their dependency.** When a rerun executes a job, the jobs that depend on it execute too, through `--depends-on` or `--depends-on-finished`, so that they do not keep results computed from the dependency's earlier output. Discussed after the CLI agent trials, where an agent wanted to rerun one array task alone. There is no option to skip dependents: the user judged that options for this edge case would not be used. Previews show why a dependent executes (`depends_on_rerun=NAME`), and the `--job-id` help says so.

## Gaps found by the trial

Each gap is a shared capability first and an interface second.

| Gap | Trial evidence | Shared capability |
| --- | --- | --- |
| Failure grouping | 300 tasks, 84 failures: `--report --failed` 108 KB; the decision is three groups | Group a run's failed jobs by cause: diagnosis rule when one matched, otherwise exit code and a normalized error line. Each group has a count, its members, one representative excerpt, and the IDs to drill down. |
| Relevant excerpts | 240 of 466 report lines were progress output from log tails | Pick excerpt lines by relevance: diagnosis evidence and its context, then stderr, then the tail. Disclose what was omitted. |
| Cause-aware comparison | `lineage` says "still failing" without saying whether the cause changed | Add each side's cause (from the same grouping key) to compared jobs, and distinguish "same cause" from "different cause". |
| Incremental progress | `wait` returns only when the run ends | Report changes since a cursor, and allow returning at the first new failure. |
| Discovery with results | `show` lists same-named projects without results; its hint and `jobs` silently assume the default basedir | Add the last run's result to the project list, make hints include the basedir when needed, and have `jobs` state its scope. |
| Effective settings | A failed `change` went unnoticed; `retry` output does not show the timeout it used | Show per-job effective settings (timeout, retry) in run output and JSON. |
| Timeout diagnosis | A timed-out job reports "timed out" and "No known rule matched" together | Treat rotari's own timeout as a known cause in diagnosis. |

## Shared behavior map

Agent-facing work must reach these owners rather than re-implement their rules. Extend this table as capabilities are added; it doubles as the parity matrix between CLI, Web, and MCP.

| Capability | Owning package(s) | CLI / Web entry | MCP |
| --- | --- | --- | --- |
| Failure grouping | `runlineage` (`FailureGroups`, beside `SummarizeDiagnoses`) | `show` run view and `--json`, `lineage RUN`, Web run summary (done in M1) | M4 |
| Excerpt selection | `report` | `show --report`, Web report endpoints | M4 |
| Diagnosis | `diagnose` | `diagnose`, `show` | via `report` |
| Run comparison | `runlineage` | `lineage` | M4 |
| Incremental progress | `projectrun` / `runview` (decide in M3) | `wait` | M4 |
| Basedir discovery | `basedirregistry` | `show`, `show --basedirs`, Web | M4 |
| Recent jobs across projects | `joblist` | `jobs`, Web jobs page | M4 |
| Project resolution | `resolve` (`RegisteredRun` for MCP), `state` | all commands | done in M0 (run ID only) |
| Job result / status | `jobstatus` | `show`, Web `loadWebJobs` | via shared functions |
| Result selection and `--filter-*` | `jobfilter` (`Filter.Selects`, `Filter.SelectsArray`) | `show`, `copy`, `run`, `retry` | via shared functions |
| Run snapshot and display | `runview`, `runregistry` | `show`, Web | via shared functions |
| History / log search | `web` (`SearchHistory`) | Web history search | open |
| Readiness | none yet (`cmd/rotari/check.go`) | `check` | M5, after extraction |
| Export | `workflow` + `cmd/rotari/export.go` | `export` | M5, after extraction |
| Import | `workflow` | `import` | M6 |
| Queue edits, run deletion | `queueops` | `add`, `change`, `remove`, `copy`, `delete` | M6 |
| Run lifecycle | `projectrun` | `run`, `retry` | M7 |
| Job control | `jobcontrol` | `cancel`, `suspend`, `resume` | M7 |
| History cleanup | none yet (`cmd/rotari/gc.go`, `reset.go`) | `gc`, `reset` | M7 |

## Milestones

Each milestone is checked by repeating the trial's scenario: triage of the mixed-failure `labA` fixture and the 300-task `labC` fixture, and the fix loop. Record call counts and output sizes in a new trial note and compare them with the [first trial](agent-trial-2026-10-02.md).

### M0: Retire `rotari-agent` and clean up the prototype (done)

- Remove `cmd/mcp/agent`; its output equals `rotari show --report`.
- Make `rotari-mcp` resolve the job through the master directory's registry instead of taking a raw basedir path.
- Structured result fields beyond the identity (project, run, job) are deferred to M4, where they come from the M1-M3 shared capabilities rather than from a second projection inside the adapter.
- Make `rotari-mcp` exit non-zero when the server fails.
- Update `docs/ARCHITECTURE.md` for the `cmd/mcp` and `internal/mcp` entries.

### M1: Failure grouping (done)

Add the shared grouping and use it in `show` for a run (human output and `--json`), then in the Web run page.

Done when, for the `labC` fixture, one `show` call answers which causes failed, how many tasks each, which tasks, and a representative error per cause, in under 2 KB of human output; and the JSON form carries the same groups with drill-down IDs. Grouping must be evaluated per job and then combined (array tasks, matrix members, carried results), never decided on an array's aggregate result.

Outcome ([measurements](agent-trial-2026-10-02-m1.md)):

- The grouping lives in `runlineage`, which already counted diagnoses for `lineage RUN` and the Web run summary; the first trial had missed that `lineage RUN` existed. A cause is a recorded block, cancellation, or timeout, else the latest saved rule diagnosis, else the `model.FailureKinds` kind, so no new error-line normalization was needed.
- `lineage RUN` answers the done question for `labC` in 1.5 KB. `show -r RUN` prints the same groups but stays at 77 KB because its 300-row job table comes first, so the criterion is met by `lineage RUN`, not by `show`. Making the compact view the obvious first call moved to M3.
- `show --json` and `lineage RUN --json` carry the groups with every member ID (6.9 KB for `labC`). Contract CLI-4 and `TestFailureGroupsAgreeAcrossViews` require `show`, `lineage RUN`, and the Web API to agree.

### M2: Relevant excerpts and cause-aware comparison (done)

- Excerpt selection by relevance in `report`, used by `show --report` and the Web report endpoints.
- Timeout as a known diagnosis cause in reports. Failure groups already classify timeouts (M1), but `show --report` still says "No known rule matched" for them.
- `lineage` comparison reports each side's cause and whether it changed.

Outcome ([measurements](agent-trial-2026-10-02-m2.md)):

- Reports keep the lines around each saved diagnosis's evidence plus the last 20 lines when the evidence is in the log, and the last 100 lines otherwise. Short fixture logs shrink only about 10% (`labC` `--report --failed` 108.4 KB to 97.3 KB); the change matters for long logs whose cause is far from the end.
- A "Job timeout reached" rule matches only rotari's own timeout line and error.
- `lineage RUN_A RUN_B` shows each run's cause (`CAUSE` column; `from_cause`, `to_cause`, `cause_changed` in JSON). The fix loop's mistake from the first trial, a timeout change that did not apply, now shows as `train[12] still failing timeout` in one call.
- Contract CLI-4 was not extended to comparisons, which have package tests but no conformance check.

### M3: Discovery and progress (done)

- Project list with last results; basedir-correct hints; `jobs` states its scope.
- Make the compact run summary the obvious first call: an agent that starts with `show -r RUN` reads the whole job table before the failure groups. Options include pointing to `lineage RUN` early in `show` output or a summary-only `show` view.
- Incremental progress for `wait` (changes since a cursor, return at first failure).
- Effective per-job settings in run output.

Outcome ([measurements](agent-trial-2026-10-02-m3.md)):

- The project list shows `LAST RESULT` (for example `failed 84/300`), and its hints work for projects outside the default state directory. Contract CLI-5 covers this, and it resolved the ISSUES entry. `jobs` names the state directory and window it searched.
- `show -r RUN` prints a `Failure summary:` line with the exact `rotari lineage` command before its job table.
- `wait --until-failure` returns at the first failure with no retry left (contract RUN-6). In the trial it returned after 1 s instead of after a 20 s job. A cursor of changes since the last call was not built: returning at the first final failure covered the trial's need, and the run's state is cheap to re-read with `lineage RUN`.
- Effective per-job settings were not added. The first trial's failure was an ignored `change` error, which `lineage RUN RUN2` now exposes as `still failing ... timeout`, and `show --json` already carries each job's settings in `commands`.
- `rotari guide`, the agent entry point, now leads with `lineage RUN_ID`, `show -j ATTEMPT_ID --report`, and `wait --until-failure`. It no longer suggests `--failed-logs` or the broken positional `--report` form.

### M4: Read-only MCP tools (done)

Expose M1-M3 through a small set of task-shaped MCP tools for clients without a shell. Done when an MCP-only agent completes the trial's triage and comparison with at most a documented number of calls and comparable output size to the CLI path, against a master directory with a same-named project in two basedirs.

Outcome ([measurements](agent-trial-2026-10-03-m4.md)):

- Tools: `rotari_list_projects`, `rotari_run_summary`, `rotari_get_job_info`, and `rotari_compare_runs`, each documented in `docs/MCP.md` with its CLI equivalent. The MCP-only trial used four calls and about 6 KB, plus a one-time 7 KB of tool schemas.
- The shared functions they need moved out of `cmd/rotari` first: `runview.RunsByStart`, `PreviousRun`, and `Summary`; `project.Overviews` and `CountRuns`; and `basedirregistry.Discover` and `Ref`. The CLI now calls the same functions, so each rule still has one implementation.
- Paths: results carry `basedir_ref` (the registry key) and `basedir_name`, never an absolute path. Evidence lines are redacted by `report.RedactPatterns`, the pattern part of the report's redaction. Redaction of known per-run values (cwd, hostname) still happens only in reports.
- Read-only: the MCP list does not remove stale locks or migrate registries, which the CLI's `show` does.
- `wait --until-failure` is not an MCP tool, because a tool call that blocks for a long time is a poor fit for MCP clients. It remains a CLI feature.

### M5: Read-only operations (done for `check`; `export` moved to M6)

Extract `check` (structured findings) and `export` (bounded or paged manifest) from `cmd/rotari` into shared packages, then expose them. Neither changes state.

Outcome:

- `check` moved into `projectrun.Runner.Check`, which `rotari check` calls; the `--deep` host checks are passed in as an optional function. `rotari_check_project` exposes it without `--deep`. It names the project by `basedir_ref` and name; `basedirregistry.Find` resolves the reference, and error messages hide the basedir path. In the MCP trial script it answered in 135 bytes.
- `export` is not exposed, and not extracted yet. A run's manifest holds job environment values, absolute working directories, and commands, which the principles keep out of default results. A redacted manifest could not be imported, and its main use is to be edited and imported. Which details an exported manifest may carry, and who approves an import, are M6 decisions. The extraction will be done then, when it has a caller.

### M6: State-changing queue and workflow operations (done for import, export, and run start; MCP queue edits remain)

`import`, queue edits (`add`, `change`, `remove`, `copy`), and run deletion, each with a separate preview and apply step. Do not start until the preview/apply contract, authorization, audit, idempotency, and recovery behavior are designed. Extract `export` from `cmd/rotari` with it, and decide what an exported manifest may carry through MCP (environment values, working directories, commands).

Outcome:

- Every CLI command that changes a project takes `--dry-run` and `--if-revision` (contract CLI-7).
- The MCP server is `rotari mcp`, in the same binary, so it can start a supervisor; `rotari-mcp` is removed.
- MCP previews (`rotari_preview_import`, `rotari_preview_run`) are read-only tools that return a revision. The writes (`rotari_import`, `rotari_start_run`) require it, and the MCP client's per-tool permission is the approval (contract MCP-1). Run start was brought forward from M7 because the trials need it.
- `export` moved into `workflowstate.LoadSettledRun`. `rotari_export_run` returns a redacted view (environment values, executor options, and detected paths); the MCP import tools refuse it, and the full manifest stays with `rotari export` (contract MCP-2).
- Not yet exposed through MCP: `add`, `change`, `remove`, `copy`, and `delete`. Their CLI forms are guarded already.

### M7: Execution and destructive operations (done; `gc` not exposed)

`run` / `retry` (asynchronous start returning a stable run identity), progress inspection, `cancel` / `suspend` / `resume`, and `gc` / `reset`. Destructive operations are individually named tools, never reachable through a read.

Outcome:

- Run start came with M6 (`rotari_start_run`), and waiting with `rotari_wait_run` (MCP-3).
- Job control (MCP-4):
  - `rotari_preview_job_control` lists the jobs that cancel, suspend, or resume would reach, chosen as the CLI chooses them.
  - `rotari_cancel`, `rotari_suspend`, and `rotari_resume` act only while the named run is still the active run. Only cancel is annotated as destructive.
  - A run ID is the guard, not a revision, because a running run changes its state continuously.
- Reset (MCP-5):
  - `rotari_preview_reset` and `rotari_reset` use the new `project.Reset`, which `rotari reset` also calls.
  - The reset applies at the previewed revision.
  - An interrupted run is recovered only with `recover_interrupted`, and the preview reports whether its jobs may still be running.
- `gc` is not exposed. It removes stale registry entries for directories that no longer exist; its output is their absolute paths, and an agent has nothing to decide there. It stays a CLI maintenance command.

## Current status

- M0 is done. `rotari-agent` is removed. `rotari_get_job_info` takes only `run_id` and `job_id`; `rotari-mcp` locates the run through its master directory's run registry and exits non-zero on failure.
- The [agent trial](agent-trial-2026-10-02.md) (2026-10-02) established the gaps above. The CLI issues it found are recorded in [ISSUES.md](../ISSUES.md): `show --json` ignoring `--failed`, a positional run ID rejected with `--json`, an array job name accepted as an unresolvable dependency, and a `show` hint missing the basedir.
- M1 is done: failures are grouped by cause in `show`, `lineage RUN`, and the Web run summary (contract CLI-4).
- M2 is done: evidence-aware report excerpts, a timeout diagnosis rule, and failure causes in `lineage` comparisons.
- M3 is done: project list results and working hints (CLI-5), `jobs` scope, a failure-summary pointer in `show`, `wait --until-failure` (RUN-6), and an updated `rotari guide`.
- M4 is done: four read-only MCP tools over the shared functions, with no absolute paths in results.
- M5 is done for `check` (`rotari_check_project`); `export` moved to M6.
- M6 CLI half is done: `add`, `change`, `copy`, `delete`, `import`, `remove`, `reset`, `run`, and `retry` take `--dry-run` and `--if-revision` (contract CLI-7), `check` reports the revision, and `gc` applies by default with `--dry-run`.
- M6 MCP half is done for import, export, and run start (MCP-1, MCP-2), served by `rotari mcp`.
- The [M6 agent trial](agent-trial-2026-10-03-m6.md) fixed and reran a project through MCP alone. It found and fixed a run preview that left out whole-array tasks, and a run summary that could not follow a started run (MCP-3). A project that has only been added is still unreachable (ISSUES.md).
- After the trial: `add` registers its basedir (`c24dc25`), and `rotari_wait_run` (MCP-3) replaced polling, so following the trial's run took one call instead of 31. This covers the waiting part of M7's progress inspection.
- Import plans and comparisons are summarized by default (`7922a08`). The write scenario now takes about 11 KB of results. Tool definitions cost 7.4 KB of model context; their output schemas are not part of it.
- M7 is done with the user's go-ahead: job control (MCP-4) and reset (MCP-5); `gc` stays CLI-only. The server has 17 tools, whose model-facing definitions take 11.3 KB, plus 1 KB of instructions.
- The [M7 agent trial](agent-trial-2026-10-03-m7.md) stopped a hanging job and recovered a crashed run through MCP alone. It found and fixed two defects that the CLI shares:
  - a cancelled pending job still started (CAN-6);
  - reset never warned that an interrupted run's jobs might still run (SAFE-7).
- Carried jobs now read as carried from the start of their run (DUR-7, `12b5395`): a run records its carried results in `carried.json` before dispatching, as the user chose.
- MCP summaries list 10 jobs per failure group and comparisons 20 changed jobs by default (`82b1769`). labC's summary fell from 7.3 KB to 3.7 KB. A comparison still repeats a definition change for each listed task, which only matters below the limit.
- Real agents use the CLI, because the user cannot connect MCP servers. Four [CLI agent trials](agent-trial-2026-10-03-cli-agent.md) drove the fixes that followed:
  - the guide and help, the missing-project error, and relative state directories;
  - `lineage` grouping and hidden-job names, previews that say why and what changes, and the async start hints;
  - the executor display, job names in results, and dependency ranges.
  - The task fell from 27 calls and about 140 KB to about 21 calls.
- Next step:
  - `retry`'s spec (ISSUES.md), with the CLI option interaction work.
  - A fifth trial that measures the rerun steps, once the agent's writes can be allowed.

## Open decisions

- Output size limits: groups and comparisons now have defaults (10 members, 20 changed jobs). Whether to collapse a definition change repeated over an array's tasks is open.
- The cursor format for incremental progress and how long a cursor stays valid. `rotari_wait_run` blocks for at most 300 seconds (default 30); revisit if MCP clients time out sooner.
- Which command and configuration details are safe and useful to return.
- Whether Web history search (`web.SearchHistory`) should back an MCP log-search tool.
- Where redaction is owned once outputs other than `report` return paths, hostnames, or commands.
- For M7: audit of MCP writes; retries after a transport timeout (a write repeated at the old revision is refused, so the client must re-preview); cancellation; partial failure.

## Validation

- Shared capabilities are tested in their owning packages with table tests whose fixtures tell cases apart: distinct exit codes, more than one failing task per cause, causes with and without a matching rule, array tasks, matrix members, carried and executed results, older attempts.
- CLI, Web, and MCP each get a test that they present the shared result; do not duplicate the domain suite per interface.
- Add conformance rows for user-visible CLI and Web API behavior, and update `contracts/README.md` when a contract changes.
- Test that unsupported selectors fail, truncation is disclosed, ambiguous selectors return candidates, and sensitive fields are absent by default.
- Repeat the agent trial after each milestone and record the result.
- Run focused package tests, `go test ./conformance` when a shared rule is touched, then `scripts/check.sh`.
