# Plan: Run Visibility and Client Disconnect Behavior

Created: 2026-10-08
Status: Proposed; behavior decisions remain open

## Purpose

Make runs that outlive their initiating CLI client easy to discover and inspect, and avoid leaving local jobs running unnoticed when a client or supervisor disappears. The workflow should distinguish an intentional detach from an explicit cancellation and from an unexpected process failure.

This plan records product direction discussed before implementation. It does not authorize changing runtime behavior until the open decisions below are resolved.

## Agreed direction

### Noun commands for collection views

Adopt distinct plural commands for listings:

| Command | Responsibility |
|---|---|
| `rotari basedirs` | List registered state directories. |
| `rotari projects` | List projects and their overview/status. |
| `rotari runs` | List runs, including detached or otherwise background runs. |
| `rotari jobs` | List jobs and their execution status. |
| `rotari show TARGET` | Show details for a project, run, job, or attempt. |

`rotari projects` takes over the project-listing view currently shown by bare `rotari show` when no project is selected. `show` is a detail command, not a general list command. If normal location resolution selects one project, bare `rotari show` may continue to show that project's details; if no unique project is selected, it should explain how to use `rotari projects`. The exact no-argument default must be specified and tested.

There is no compatibility requirement for unreleased CLI forms. Do not add aliases solely to preserve `show --basedirs` or other old listing syntax unless implementation discovers a concrete need.

### Client disconnect and supervisor failure

- Ctrl-C is an explicit cancellation request.
- Ctrl-D is an intentional detach.
- Ctrl-Z suspends the client; it does not stop the run.
- A run that continues after its initiating client is gone must be readily discoverable and inspectable.
- The preferred direction is to consider treating an unexpected run-client disconnect as detach rather than cancellation, once background-run visibility is available. The default behavior is not yet approved: distinguish Ctrl-C from EOF, client timeout/termination, and supervisor death before changing it.
- A killed supervisor is not automatically restarted. Its local jobs may outlive it and write their own attempt status. Existing `rotari cancel --job-id JOB_ID` was manually verified to send TERM to an orphan local job's process group while its stale run lock remains; it does not finalize the interrupted run. This is useful for stopping stray work, not run recovery.

## Goals

- Let a user or coding agent find all relevant active/background runs without remembering a project name or run ID.
- Make attached, detached, and interrupted status distinguishable where the system can know it reliably.
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
2. **Client attachment visibility:** What precisely does `rotari runs` report: `attached`, `detached`, `interrupted`, or only run/project state? Is attachment state reliably observable from the supervisor's pipe lifecycle, or must it be persisted?
3. **Listing scope:** Should `runs` list only active/background runs by default, or include recent completed history with a time window? Which selectors and output formats are needed initially?
4. **Project default:** Should bare `show` use an explicitly configured project only, or any unambiguous project resolved through existing defaults/discovery?
5. **Orphan stopping:** Is existing `cancel -p PROJECT --job-id JOB_ID` sufficient after supervisor death, or is a dedicated interrupted-run stop operation needed? What should happen for scheduler/SSH jobs, remote-host mismatch, stale/reused local PIDs, and TERM-resistant descendants?
6. **`server list`:** Keep it as supervisor diagnostics, separate from user-facing run listing, unless implementation shows the views can be unified without losing meaning.

## Proposed implementation phases

1. **Confirm lifecycle semantics.** Trace EOF, Ctrl-C, Ctrl-D, process timeout/termination, and supervisor death through the pipe protocol. Specify which cases cancel, detach, or interrupt. Do not conflate client and supervisor signals.
2. **Add collection commands.** Implement `basedirs`, `projects`, and `runs` using existing shared resolution and listing logic; keep `jobs` as the job-level view. Refactor bare `show` into selected-target detail and update generated CLI schema/help, docs, and examples.
3. **Expose background run state.** Have `runs` report enough information to find a run and decide whether it is still active, detached, interrupted, or complete. Prefer deriving attachment from authoritative live coordination state; persist it only if it cannot be reliably derived. Define behavior for stale locks and remote hosts.
4. **Adjust disconnect policy, if approved.** Once detached runs are discoverable, implement the chosen default/option. Preserve Ctrl-C cancellation and Ctrl-D detach. Make client exit status and printed wait/show hints unambiguous.
5. **Document orphan cancellation.** State that job-level cancel can signal eligible orphan jobs but does not finalize an interrupted run. Define and test any stronger stop/escalation mechanism separately.

## Tests and validation

- Table-test bare `show` with no project, explicit project defaults, a sole project, multiple projects, and run/job selectors.
- Exercise each list command in isolated state: empty state, multiple basedirs/projects, active async run, detached sync run, interrupted run, and settled history.
- Test attached/detached/interrupted classification across local and remote-host cases, including stale and malformed locks.
- Use subprocess tests to distinguish Ctrl-C, Ctrl-D, EOF, SIGTERM, and SIGKILL of the CLI client; assert job effects and run finalization separately.
- Verify a killed supervisor leaves the run interrupted, that local orphan job cancellation stops the process group and records cancellation where the wrapper survives, and that the run is not falsely finalized.
- Add conformance coverage for user-visible CLI behavior; update contracts and contract-status coverage alongside any behavior change.
- Run focused CLI/server/jobcontrol tests first, then affected conformance tests, formatting, `scripts/check.sh --short`, and `scripts/check.sh`.

## Current evidence

- `TestJobOutlivesKilledSupervisor` passes and confirms local work can outlive a SIGKILLed supervisor and the project becomes interrupted.
- An isolated manual trial confirmed `rotari cancel -p PROJECT --job-id JOB_ID` can terminate an orphan local job's process group after the supervisor is killed; the run remains interrupted.
- Existing docs specify Ctrl-C cancellation, Ctrl-D detach, and Ctrl-Z suspend for synchronous runs. Unexpected client disconnect currently requests cancellation.
- No code changes for this plan have been made.
