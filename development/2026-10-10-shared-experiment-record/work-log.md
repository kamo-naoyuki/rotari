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

## Reconstruction trial 2

- `a2ddb274` (2026-10-10 20:26:51 +0900): [trial-2026-10-10-reconstruction-2.md](trial-2026-10-10-reconstruction-2.md).

**Change:** the trial report and the plan's status. Documentation only.

**Reason:** to measure source revisions and run notes against trial 1, with the same fixture, task, and variants.

**Plan impact:**
- Both agents wrote a note at every run start and a conclusion afterwards, and committed their code changes; Q2, Q3, Q5, and Q6 became answerable from the record, except where an agent ran uncommitted code.
- New finding: a preview and the run started after it can differ, because `--if-revision` guards only the project's state. rA previewed a filtered retry, started it without the filter, and its note no longer matched what ran. A replay of rA's command sequence at `24205def` and at `6e727399` executed the same jobs, so it was the agent's omitted option, not a selection bug.
- New finding: a clean git source prints no "clean", and a reconstruction agent read that as dirty state not being recorded.

**Validation:** four headless `claude -p` agents on fixtures built at `6e727399`; the reconstruction agents read nothing outside the work directory. The replay script and transcripts are in the session scratchpad.

**Remaining:** the report's findings 1 (plan-guarding preview revision, a decision for the user), 2 (show clean sources), and the open trial 1 findings 3 and 5.

## A run preview's revision guards its plan

- `23f2c9df` (2026-10-10 20:51:42 +0900): refuse a run that would execute other jobs than its preview.

**Change:**
- `internal/projectrun/plan_revision.go`: `PlanRevision` (project revision, a dot, and a hash of the executed and carried job IDs and the source run), `CheckRunProjectRevision`, `CheckRunPlanRevision`, and `ErrPlanChanged`, whose message names the jobs the start would execute.
- `PreviewRun` returns the plan revision and refuses a plan that differs from a given one; the supervisor's `prepareRun` checks the project part before planning and the plan part after it, so the CLI and MCP starts share the rule.
- CLI-7 and MCP-1, `docs/RECOVERING.md`, `docs/ARCHITECTURE.md`, and the agent guide.
- Tests that pinned "a run preview's revision equals check's" now expect it as the prefix: `TestRunPreviewMatchesTheRun` (which also checks a refused start of another plan), `TestMCPWritesApplyOnlyAtThePreviewedRevision`, two `internal/mcp` and two `internal/projectrun` tests, and the revision-line regexes of two conformance packages.

**Reason:** trial 2 finding 1; the user agreed after the `--if-revision` design was explained.

**Plan impact:** decision recorded in [plan.md](plan.md).

**Validation:** new `TestPlanRevisionTellsPlansApart`, `TestCheckRunPlanRevision`, and `TestStartRunRefusesAnotherPlanThanItsPreview`; a manual replay of trial 2's dropped filter is refused with the jobs it would run; `scripts/check.sh` with the race detector passed.

**Remaining:** trial 2 finding 2 (show clean sources) and trial 1 findings 3 and 5.

## Clean sources and unexpanded variables

- `f8bcdc8e` (2026-10-10 21:04:12 +0900): a git source says `(clean)` or `(uncommitted changes)`.
- `0aff4bf6` (2026-10-10 21:04:12 +0900): `add` and a command-replacing `change` warn about arguments such as `$LR` that no shell expands (CLI-25).

**Change:**
- `model.FormatSourceRevision` adds `(clean)` to a git revision without uncommitted changes; RUN-15, `docs/INSPECT.md`, and the source conformance tests follow.
- `model.UnexpandedVariables` finds `$NAME` and `${NAME}` arguments of a command that `sh`, `bash`, `dash`, `zsh`, or `ksh` does not run with `-c`; `queueops` warns once per add or change through `Editor.Warn`, which `change` now sets too. The agent guide no longer calls the commands shell commands and says how to pass matrix values; `docs/FAQ.md` mentions the warning.

**Reason:** trial 2 finding 2, and trial 1 finding 5, which recurred for every agent in both trials.

**Plan impact:** none beyond closing those findings.

**Validation:** `TestUnexpandedVariables`, `TestAddAndChangeWarnAboutUnexpandedVariables`, conformance `TestAddWarnsAboutUnexpandedVariables`, and the updated `TestRunRecords*`; `scripts/check.sh` with the race detector passed.

**Remaining:** comparison across projects (trial 1 finding 3), and a trial where the user reads an agent's runs.
