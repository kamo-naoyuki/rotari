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

### M2: Relevant excerpts and cause-aware comparison

- Excerpt selection by relevance in `report`, used by `show --report` and the Web report endpoints.
- Timeout as a known diagnosis cause in reports. Failure groups already classify timeouts (M1), but `show --report` still says "No known rule matched" for them.
- `lineage` comparison reports each side's cause and whether it changed.

### M3: Discovery and progress

- Project list with last results; basedir-correct hints; `jobs` states its scope.
- Make the compact run summary the obvious first call: an agent that starts with `show -r RUN` reads the whole job table before the failure groups. Options include pointing to `lineage RUN` early in `show` output or a summary-only `show` view.
- Incremental progress for `wait` (changes since a cursor, return at first failure).
- Effective per-job settings in run output.

### M4: Read-only MCP tools

Expose M1-M3 through a small set of task-shaped MCP tools for clients without a shell. Done when an MCP-only agent completes the trial's triage and comparison with at most a documented number of calls and comparable output size to the CLI path, against a master directory with a same-named project in two basedirs.

### M5: Read-only operations

Extract `check` (structured findings) and `export` (bounded or paged manifest) from `cmd/rotari` into shared packages, then expose them. Neither changes state.

### M6: State-changing queue and workflow operations

`import`, queue edits (`add`, `change`, `remove`, `copy`), and run deletion, each with a separate preview and apply step. Do not start until the preview/apply contract, authorization, audit, idempotency, and recovery behavior are designed.

### M7: Execution and destructive operations

`run` / `retry` (asynchronous start returning a stable run identity), progress inspection, `cancel` / `suspend` / `resume`, and `gc` / `reset`. Destructive operations are individually named tools, never reachable through a read.

## Current status

- M0 is done. `rotari-agent` is removed. `rotari_get_job_info` takes only `run_id` and `job_id`; `rotari-mcp` locates the run through its master directory's run registry and exits non-zero on failure.
- The [agent trial](agent-trial-2026-10-02.md) (2026-10-02) established the gaps above. The CLI issues it found are recorded in [ISSUES.md](../ISSUES.md): `show --json` ignoring `--failed`, a positional run ID rejected with `--json`, an array job name accepted as an unresolvable dependency, and a `show` hint missing the basedir.
- M1 is done: failures are grouped by cause in `show`, `lineage RUN`, and the Web run summary (contract CLI-4).
- Next step: M2, relevant excerpts and cause-aware comparison.

## Open decisions

- Output size limits and defaults for groups, members, and excerpts.
- The cursor format for incremental progress and how long a cursor stays valid.
- Which command and configuration details are safe and useful to return.
- Whether Web history search (`web.SearchHistory`) should back an MCP log-search tool.
- Where redaction is owned once outputs other than `report` return paths, hostnames, or commands.
- For M6/M7: the preview, confirmation, apply, and result-reporting sequence; authorization and audit; idempotency and retries after a transport timeout; asynchronous run handles; cancellation; partial failure.

## Validation

- Shared capabilities are tested in their owning packages with table tests whose fixtures tell cases apart: distinct exit codes, more than one failing task per cause, causes with and without a matching rule, array tasks, matrix members, carried and executed results, older attempts.
- CLI, Web, and MCP each get a test that they present the shared result; do not duplicate the domain suite per interface.
- Add conformance rows for user-visible CLI and Web API behavior, and update `contracts/README.md` when a contract changes.
- Test that unsupported selectors fail, truncation is disclosed, ambiguous selectors return candidates, and sensitive fields are absent by default.
- Repeat the agent trial after each milestone and record the result.
- Run focused package tests, `go test ./conformance` when a shared rule is touched, then `scripts/check.sh`.
