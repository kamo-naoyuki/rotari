# Plan: Run Visibility and Client Disconnect Behavior

Created: 2026-10-08
Status: Listing commands and detail-only `show` implemented; job/client status visibility and disconnect behavior remain open

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
- The preferred direction is to consider treating an unexpected run-client disconnect as detach rather than cancellation, once background-run visibility is available. The default behavior is not yet approved: distinguish Ctrl-C from EOF, client timeout/termination, and supervisor death before changing it.
- A killed supervisor is not automatically restarted. Its local jobs may outlive it and write their own attempt status. Existing `rotari cancel --job-id JOB_ID` was manually verified to send TERM to an orphan local job's process group while its stale run lock remains; it does not finalize the interrupted run. This is useful for stopping stray work, not run recovery.

### Status dimensions

Do not collapse run lifecycle, per-job execution, and client attachment into one status. Display them as separate fields where applicable.

| Dimension | Values / display | Meaning |
| --- | --- | --- |
| Run lifecycle | `running`, `interrupted`, `finished`, `failed`, `incomplete` | Whether the coordinator is active and whether the run has a valid summary. `finished` and `failed` are settled run outcomes; `incomplete` means no valid summary is available. |
| Job execution | `not started`, `waiting (recorded)`, `running (recorded)`, `success`, `failed`, `cancelled`, `blocked`, `unknown` | Best-effort interpretation of the latest attempt and recorded result. `waiting (recorded)` and `running (recorded)` are last-observed states, not proof of current executor state. Missing status alone must not be presented as definitely `not started`; use `unknown` unless there is positive evidence. A carried result should be identified as carried rather than as work executed in this run. |
| Client connection | `attached`, `async (detached)`, `detached (Ctrl-D)`, `disconnecting/cancelling`, `unknown` | Connection between the initiating synchronous run/retry client and its supervisor. Async and Ctrl-D both mean no attached progress client, but retain their reason in the label. Unexpected disconnect remains distinct because the current behavior requests cancellation. |

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

1. **Unexpected disconnect default:** Should a synchronous run whose client receives EOF or is terminated detach automatically, or keep the current cancellation default? Can users select the policy per invocation/configuration?
2. **Orphan stopping:** Is existing `cancel -p PROJECT --job-id JOB_ID` sufficient after supervisor death, or is a dedicated interrupted-run stop operation needed? What should happen for scheduler/SSH jobs, remote-host mismatch, stale/reused local PIDs, and TERM-resistant descendants?
3. **`server list`:** Keep it as supervisor diagnostics, separate from user-facing run listing, unless implementation shows the views can be unified without losing meaning.

## Proposed implementation phases

1. **Confirm lifecycle semantics.** Trace EOF, Ctrl-C, Ctrl-D, process timeout/termination, and supervisor death through the pipe protocol. Specify which cases cancel, detach, or interrupt. Do not conflate client and supervisor signals.
2. **Add collection commands.** Complete: `basedirs`, `projects`, `runs`, and `jobs` use existing registry/project state; listing commands scan all known basedirs by default, and `--basedir` narrows `runs`/`jobs`. The obsolete `--all-basedirs` option was removed. `runs`/`jobs` default to a one-day history window while retaining active work. Bare `show` resolves one project to its detail view and directs ambiguous selection to `projects`. CLI schema/help, generated Python schema, user docs, contracts, and CLI/conformance tests were updated.
3. **Expose job and client status.** Not implemented. Read each job's persisted attempt/result using the shared job-status resolution; display terminal outcomes, recorded nonterminal phase, and unknown distinctly. Persist the initiating client's sync/async mode and connection transitions in run-scoped state. Provide a shared read path for `runs`, `show`, and the corresponding Web/API projection. Ignore stale attachment records when the supervisor is not verifiably alive; remote-host cases that cannot be checked are `unknown`.
4. **Adjust disconnect policy, if approved.** Deferred and out of scope for status visibility. Keep the current unexpected-disconnect cancellation behavior for this work. Preserve explicit Ctrl-C cancellation and Ctrl-D detach; make resulting status text unambiguous.
5. **Document orphan cancellation.** State that job-level cancel can signal eligible orphan jobs but does not finalize an interrupted run. Define and test any stronger stop/escalation mechanism separately.

## Tests and validation

- Table-test bare `show` with no project, explicit project defaults, a sole project, multiple projects, and run/job selectors.
- Exercise each list command in isolated state: empty state, multiple basedirs/projects, active, interrupted, incomplete, and settled runs.
- Table-test client states: synchronous attached, async (detached), Ctrl-D detached, unexpected EOF/cancellation, completed, interrupted supervisor, stale/malformed state, and unverifiable remote-host supervisor.
- Test per-job display for recorded waiting/running followed by a wrapper terminal update after supervisor exit, success/failure/cancelled/blocked results, definitely-not-started jobs, missing status, and carried results. Assert that an unverified waiting/running record is labeled as recorded/best-effort, not confirmed live.
- Verify that `wait`, Web, and MCP do not falsely appear as attached run-progress clients.
- Verify `runs` and `show` share the same status interpretation; add Web/API coverage for the same projection.
- Use subprocess tests to distinguish Ctrl-C, Ctrl-D, EOF, SIGTERM, and SIGKILL of the CLI client; assert job effects and run finalization separately.
- Verify a killed supervisor leaves the run interrupted, that local orphan job cancellation stops the process group and records cancellation where the wrapper survives, and that the run is not falsely finalized.
- Add conformance coverage for user-visible CLI behavior; update contracts and contract-status coverage alongside any behavior change.
- Run focused CLI/server/jobcontrol tests first, then affected conformance tests, formatting, `scripts/check.sh --short`, and `scripts/check.sh`.

## Current evidence

- `TestJobOutlivesKilledSupervisor` passes and confirms local work can outlive a SIGKILLed supervisor and the project becomes interrupted.
- An isolated manual trial confirmed `rotari cancel -p PROJECT --job-id JOB_ID` can terminate an orphan local job's process group after the supervisor is killed; the run remains interrupted.
- Existing docs specify Ctrl-C cancellation, Ctrl-D detach, and Ctrl-Z suspend for synchronous runs. Unexpected client disconnect currently requests cancellation.
- The list-command/show phase is complete. Focused CLI tests, affected conformance suites, read-only flag pairs, contract/golden checks, pre-commit, and `scripts/check.sh --short` passed. The final `scripts/check.sh` passed with go vet, all Go tests, and race tests (exit 0). Python tests passed against the freshly built CLI (64 passed); generated references and README synchronization checks passed, as did the strict MkDocs build.
