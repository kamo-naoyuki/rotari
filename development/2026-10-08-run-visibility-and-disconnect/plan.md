# Plan: Run Visibility and Client Disconnect Behavior

Created: 2026-10-08
Status: Run, job, and initiating-client status visibility implemented and validated

Implementation history: [work-log.md](work-log.md).

## Purpose

Make runs that outlive their initiating CLI client easy to discover and inspect, and avoid leaving local jobs running unnoticed when a client or supervisor disappears. The workflow should distinguish an intentional detach from an explicit cancellation and from an unexpected process failure.

This plan records product direction discussed before implementation. It does not authorize changing runtime behavior until the open decisions below are resolved.

## Agreed direction

### Noun commands for collection views

Adopt distinct plural commands for listings:

| Command | Responsibility |
| --- | --- |
| `rotari basedirs` | List registered state directories. |
| `rotari projects` | List projects and their overview/status. |
| `rotari runs` | List runs, including detached or otherwise background runs. |
| `rotari jobs` | List jobs and their execution status. |
| `rotari show TARGET` | Show details for a project, run, job, or attempt. |

`rotari projects` takes over the project-listing view currently shown by bare `rotari show` when no project is selected. `show` is a detail command, not a general list command. Bare `show` uses normal project resolution; it shows the selected project's current run or queue and, if more than one project is possible without a selector, exits with guidance to use `rotari projects`.

`rotari runs`, `rotari jobs`, and `rotari projects` scan all known basedirs by default. `--basedir DIR` narrows `runs` or `jobs` to one state directory. `runs` and `jobs` default to a one-day completed-history window; active/interrupted runs and running jobs are always included. The obsolete `--all-basedirs` option is removed. `runs` and `show` expose run lifecycle and client connection as separate status dimensions.

There is no compatibility requirement for unreleased CLI forms. Do not add aliases solely to preserve `show --basedirs` or other old listing syntax unless implementation discovers a concrete need.

### Client disconnect and supervisor failure

- Ctrl-C is an explicit cancellation request.
- Ctrl-D is an intentional detach.
- Ctrl-Z suspends the client; it does not stop the run.
- A run that continues after its initiating client is gone must be readily discoverable and inspectable.
- Unexpected run-client disconnect defaults to detach; `--disconnect-action cancel` or `ROTARI_DISCONNECT_ACTION=cancel` restores cancellation. Ctrl-C remains explicit cancellation and Ctrl-D remains intentional detach.
- A killed supervisor is not automatically restarted. Its local jobs may outlive it and write their own attempt status. Existing `rotari cancel --job-id JOB_ID` was manually verified to send TERM to an orphan local job's process group while its stale run lock remains; it does not finalize the interrupted run. This is useful for stopping stray work, not run recovery.

### Status dimensions

Do not collapse run lifecycle, per-job execution, and client attachment into one status. Display them as separate fields where applicable.

| Dimension | Values / display | Meaning |
| --- | --- | --- |
| Run lifecycle | `running`, `interrupted`, `finished`, `failed`, `incomplete` | Whether the coordinator is active and whether the run has a valid summary. `finished` and `failed` are settled run outcomes; `incomplete` means no valid summary is available. |
| Job execution | `waiting (recorded)`, `running (recorded)`, `suspended (recorded)`, `success`, `failed`, `cancelled`, `blocked`, `unknown` | Best-effort interpretation of the latest attempt and recorded result. Recorded phases are last-observed states, not proof of current executor state. No positive evidence of execution yields `unknown`; the implementation does not emit `not started`. A carried result is identified as carried rather than as work executed in this run. |
| Client connection | `attached`, `async (detached)`, `detached (Ctrl-D)`, `detached (disconnect)`, `disconnecting/cancelling`, `completed`, `unknown` | Connection history of the initiating run/retry client. Async and Ctrl-D both mean no attached progress client but retain their reason. Unexpected disconnect detaches by default or records cancellation when configured. Finalized client records show completed history; interrupted or unverifiable records show unknown, never a live attachment. |

`wait`, Web, and MCP consumers are not attached run-progress clients: `wait` and Web poll/read persisted state, while MCP starts runs asynchronously. Do not count them as attached clients in this first implementation. A completed run can show its recorded launch/detach mode as history, but must not imply a live client remains attached. If the supervisor is unavailable or the recorded connection state cannot be validated, report `unknown`; do not treat a stale `attached` record as current.

Run/job status is best-effort and must not query executors for this feature. Local and scheduler wrappers can update attempt files after a coordinator exits; the display reads the latest persisted state. This can be stale, and SSH may not record terminal state after the coordinator disappears. Preserve that uncertainty in labels rather than claiming live process state.

## Goals

- Let a user or coding agent find all relevant active/background runs without remembering a project name or run ID.
- Show run lifecycle, best-effort per-job status, and initiating-client attachment as separate dimensions in `runs` and `show`.
- Distinguish `async (detached)` from `detached (Ctrl-D)` while making clear that both have no attached progress client.
- Make it straightforward to inspect a run and cancel unwanted jobs after the initiating client is gone.
- Keep explicit Ctrl-C cancellation predictable and keep intentional detach non-cancelling.
- Make the command taxonomy clear: plural nouns list; `show` inspects one selected target.

## Non-goals

- Reconnect to a live run's original progress stream or terminal.
- Automatically restart a killed supervisor or reconcile/finalize its run.
- Treat `unlock` as a process-kill operation; it only recovers project state and does not stop jobs.
- Promise that SIGTERM stops commands that ignore it or descendants that leave their process group.
- Add a separate force-kill command before defining safe process identity checks and escalation semantics.

## Open decisions

1. **Unexpected disconnect default — resolved:** Unexpected run/wait client disconnect detaches by default; CLI/environment configuration can request cancellation. Ctrl-C and Ctrl-D retain their explicit meanings.
2. **Orphan stopping:** Is existing `cancel -p PROJECT --job-id JOB_ID` sufficient after supervisor death, or is a dedicated interrupted-run stop operation needed? What should happen for scheduler/SSH jobs, remote-host mismatch, stale/reused local PIDs, and TERM-resistant descendants?
3. **`server list`:** Keep it as supervisor diagnostics, separate from user-facing run listing, unless implementation shows the views can be unified without losing meaning.

## Proposed implementation phases

1. **Confirm lifecycle semantics.** Trace EOF, Ctrl-C, Ctrl-D, process timeout/termination, and supervisor death through the pipe protocol. Specify which cases cancel, detach, or interrupt. Do not conflate client and supervisor signals.
2. **Add collection commands.** Complete: `basedirs`, `projects`, `runs`, and `jobs` use existing registry/project state; listing commands scan all known basedirs by default, and `--basedir` narrows `runs`/`jobs`. The obsolete `--all-basedirs` option was removed. `runs`/`jobs` default to a one-day history window while retaining active work. Bare `show` resolves one project to its detail view and directs ambiguous selection to `projects`. CLI schema/help, generated Python schema, user docs, contracts, and CLI/conformance tests were updated.
3. **Expose job and client status.** Implemented: `jobstatus.Job.DisplayStatus` classifies terminal, recorded nonterminal, and unknown states without querying executors. Run-scoped `client_status.json` records sync/async mode and the last connection transition. Shared `runview` projections supply lifecycle/client labels to CLI and Web; `runs` and `show` display them separately, and run/job JSON and Web API expose structured fields. An unverified supervisor makes live attachment `unknown`.
4. **Adjust disconnect policy.** Implemented before this status work: unexpected client disconnect detaches by default and can be configured to cancel. This task preserves that policy, Ctrl-C cancellation, and Ctrl-D detach while recording their reasons.
5. **Document orphan cancellation.** State that job-level cancel can signal eligible orphan jobs but does not finalize an interrupted run. Define and test any stronger stop/escalation mechanism separately.

## Tests and validation

- Table-test bare `show` with no project, explicit project defaults, a sole project, multiple projects, and run/job selectors.
- Exercise each list command in isolated state: empty state, multiple basedirs/projects, active, interrupted, incomplete, and settled runs.
- Table-test client states: synchronous attached, async (detached), Ctrl-D detached, unexpected EOF/cancellation, completed, interrupted supervisor, stale/malformed state, and unverifiable remote-host supervisor.
- Test per-job display for recorded waiting/running/suspended followed by a wrapper terminal update after supervisor exit, success/failure/cancelled/blocked results, missing or unusable status, and carried results. Assert that absent execution evidence is unknown and recorded phases are best-effort, not confirmed live.
- Verify that `wait`, Web, and MCP do not falsely appear as attached run-progress clients.
- Verify `runs` and `show` share the same status interpretation; add Web/API coverage for the same projection.
- Use subprocess tests to distinguish Ctrl-C, Ctrl-D, EOF, SIGTERM, and SIGKILL of the CLI client; assert job effects and run finalization separately.
- Verify a killed supervisor leaves the run interrupted, that local orphan job cancellation stops the process group and records cancellation where the wrapper survives, and that the run is not falsely finalized.
- Add conformance coverage for user-visible CLI behavior; update contracts and contract-status coverage alongside any behavior change.
- Run focused CLI/server/jobcontrol tests first, then affected conformance tests, formatting, `scripts/check.sh --short`, and `scripts/check.sh`.

## Current evidence

- `TestJobOutlivesKilledSupervisor` passes and confirms local work can outlive a SIGKILLed supervisor and the project becomes interrupted.
- An isolated manual trial confirmed `rotari cancel -p PROJECT --job-id JOB_ID` can terminate an orphan local job's process group after the supervisor is killed; the run remains interrupted.
- Existing CLI behavior defaults unexpected client disconnect to detach; `--disconnect-action cancel` and `ROTARI_DISCONNECT_ACTION=cancel` retain cancellation as an option.
- The list-command/show and disconnect-policy phases are complete. Job/client status visibility is implemented; focused and full validation results will be recorded after execution.
