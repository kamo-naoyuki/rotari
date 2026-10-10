# Work Log: rotari as a Shared Experiment Record

Entries group cohesive changes. Times are Git commit times.

## The plan and the first reconstruction trial

- `e4d097aa` (2026-10-10 18:04:55 +0900): the plan.
- `24205def` (2026-10-10 18:15:34 +0900): trial 1, its fixture and run scripts, and its report.

**Change:** Added [plan.md](plan.md), [trial-fixture.sh](trial-fixture.sh), [trial-run.sh](trial-run.sh), and [trial-2026-10-10-reconstruction.md](trial-2026-10-10-reconstruction.md).

**Reason:** the user wants rotari to be a platform where agents and humans share one experiment record. Earlier trials measured only the agent's cost of using rotari.

**Plan impact:** Phase 1 trial 1 confirmed that neither the code a run executed nor why it was made can be recovered from the record, and that `lineage` cannot compare runs across projects. An agent told only that rotari is installed used it for every run.

**Validation:** four headless `claude -p` agents (two experiments, two reconstructions); scores in the report. Documentation and scripts only.

**Remaining:** run notes; comparison across projects; the guide's "shell commands" wording (trial finding 5).

## Source revisions recorded with each run

- `a812b015` (2026-10-10 19:10:47 +0900): git or jj revision per run (RUN-15).

**Change:**
- New `internal/sourcerev` reads the revision of the repository holding a directory: jj (after a working-copy snapshot) before git, so a colocated repository is read as jj. Boundary rule in `internal/archtest` and `contracts/00-overview.md`.
- `model.SourceRevision`, `RunSources`, `SourceFor`, `FormatSourceRevision`, and `SourceLabel`.
- `projectrun.Runner.Execute` writes `sources.json` before dispatching, for the executed jobs' working directories only.
- `state.SaveRunSources`, `LoadRunSources`, and `AttemptSource`.
- `runlineage.CompareSources`; `Run.Sources`, `RunInfo.Sources`, and `Result.Sources`, loaded by `runview.LoadRun`.
- `show` (run header and `-j`), `lineage RUN` and `lineage A B`, the Web run page and API (`sources`, `source_labels`), and the MCP run summary and comparison, which name a repository by its last path element.
- Contract RUN-15, `docs/INSPECT.md`, `docs/CONCEPTS.md`, `docs/ARCHITECTURE.md`, `docs/MCP.md`, and the agent guide.

**Reason:** trial 1's findings 1 (code version). The user chose commit IDs over file hashes or diffs, and a jj snapshot for exactness.

**Plan impact:** Phase 2's first item is done. Run notes are next.

**Validation:**
- New tests: `internal/sourcerev` (git clean, dirty, untracked, no commits, outside a repository, jj and colocated jj), `model` and `runlineage` table tests, `TestRunToolsNameSourcesWithoutPaths`, and conformance `TestRunRecordsTheSourceItExecuted`, `TestRunRecordsUncommittedGitChanges`, `TestRunRecordsAJJWorkingCopy` (skipped without jj).
- The conformance tests first failed in the full suite because two runs in the same second sorted by run ID in the wrong order; they now take each run's ID right after it. They then passed five times in a row.
- `go test ./conformance/...` and `scripts/check.sh` with the race detector passed.
- Manual runs in a git and a jj repository showed `unknown` for a dirty git tree and `changed` with the same change ID after a jj edit.

**Remaining:** a job dispatched long after its run started may run code edited after the record was taken; `Execute` records once, before dispatch.

## Notes on runs and job attempts

- `0c6eb957` (2026-10-10 20:06:04 +0900): run and attempt notes (RUN-16).

**Change:**
- `model.RunNote`, `NoteText`, `RunNotesFor`, and `FormatRunNote`; `state.AppendRunNote` and `LoadRunNotes` over the run's `notes.jsonl`.
- `run --note` / `retry --note` pass the note through `server.Request` to `projectrun.Start`, which `Begin` records; a dry run prints it. MCP `rotari_start_run` takes `note`.
- New `rotari note RUN_ID|ATTEMPT_ID TEXT` (`cmd/rotari/note.go`) over `queueops.AddNote`.
- `runlineage.Run.Notes` and `RunInfo.Notes`, loaded by `runview.LoadRun`; `LineageEntry.CodeChange` from `CombineSourceChanges`.
- Views: `show` (run notes and job notes by job name), `show -j`, `lineage RUN`, `lineage A B` (`Note (from)` / `Note (to)`), and the run history's `CODE` and `NOTE` columns; the Web run page and API (`notes`, `note_labels`); the MCP run summary and comparison, with paths redacted.
- Python client `Rotari.note` and `Run.note`; regenerated `generated_cli.py`, `docs/CLI_REFERENCE.md`, and `docs/python-api.md`.
- Contract RUN-16, `docs/INSPECT.md`, `docs/CONCEPTS.md`, `docs/ARCHITECTURE.md`, `docs/MCP.md`, the flag-pair coverage document, and the agent guide, which asks agents to write a reason and a conclusion.

**Reason:** trial 1's finding 2 (intent and conclusion), with the user's choices: notes on runs and attempts, no notes on job definitions, no author.

**Plan impact:** Phase 2's second item is done. Trial 2 should show whether agents write notes when the guide asks, and whether Q2 and Q6 become answerable.

**Validation:**
- Unit tests in `model`, `state`, `queueops`, `runlineage`, and `mcp`; Python `test_object_api`.
- Conformance `TestRunNotes` (CLI views, errors, dry run, Web API) and `TestCLIFlagPairNote` (a pair adapter for the new command, so no generated pair is deferred). `TestCLIFlagPairInventory` and the help/schema goldens were updated for the new command and option; the golden diff holds only them.
- `scripts/check.sh` with the race detector passed. One earlier full run failed in `TestWaitJSONDisconnectWithSIGKILLCancelsRun`, which passed 20 isolated repeats on this change and on the commit before it; recorded in ISSUES.md.

**Remaining:** reconstruction trial 2.
