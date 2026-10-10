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

## Job notes behind a button on the Web run page

- `afb06017` (2026-10-10 21:32:37 +0900): a `Notes (N)` button for each job with notes.

**Change:** `web.Job.NoteLabels` (the job's notes, formatted by `model.FormatRunNote`, naming an attempt other than the latest), filled by `LoadQueueState`; `showNotes` in `web_app_logs.js` opens them in the output modal from a button beside `Output` and `Artifacts`. RUN-16 and `docs/INSPECT.md` mention it.

**Reason:** the user asked for job notes next to the job's report and log buttons. The run notes' placement is still being decided, so they stay at the top of the run page.

**Plan impact:** none.

**Validation:** new `TestWebRunPageShowsJobNotesButton` (jsdom: only the noted task has the button; it shows its two notes, naming the earlier attempt, and not the run's note); `TestRunNotes` checks the Web API's per-job labels; `scripts/check.sh` with the race detector passed.

**Remaining:** where run notes go on the Web pages (the user's decision).

## Disabled Notes button and run-only notes at the top of the run page

- `6e4d1085` (2026-10-10 21:47:35 +0900): a job without notes shows a disabled `Notes` button.
- `a60f583a` (2026-10-10 22:19:50 +0900): the run page's header lists only the notes on the run itself.

**Change:**
- `notesControl` in `web_app_core.js` renders a disabled `Notes` button, titled with how to add a note, for a job without notes, so every job row has the same buttons.
- `web.Run.NoteLabels` (the Web API run's `note_labels`) describes only the run's own notes, through `runNoteLabels`; a job's notes appear only in its `Job.NoteLabels`. The run's `notes` still carries every note. RUN-16 and `docs/INSPECT.md` follow.

**Reason:** the user asked for the button to be shown disabled when a job has no notes, and then for job notes to be left out of the top of the run page for now, since they are behind each job's button.

**Plan impact:** none. Where run notes go on the Web pages is still the user's decision; this only stops listing job notes there.

**Validation:** `TestWebRunPageShowsJobNotesButton` checks the disabled button; `TestRunNotes` now expects the run's labels to hold its two run notes only and failed against the previous loader with the job note among them; `go test ./conformance/...` and `scripts/check.sh` with the race detector passed.

**Remaining:** run notes placement on the Web (the user's decision); the browser-notification issues found while answering a question (notifications for jobs first seen when an old run is opened, and a settings lookup by the wrong key), not yet fixed or recorded.

## One run report for people and agents (Phase 3, first cut)

- `7c68174b` (2026-10-10 22:57:14 +0900): the run report gains sources, notes, and a job table, and the Web renders it as Markdown.

**Change:**
- `internal/report`: a run report lists the run's sources and its run notes, then a `## Jobs` table (job, array task, the environment variables whose values differ between the listed jobs, status, exit code, first and last log line), then each job's section with that job's notes; a job report gains the sources and the job's notes. `loadRun` reads `sources.json` and `notes.jsonl`. `web.Job.Environment` (left out of the Web API JSON, since `--env` values may be secrets) carries each job's environment.
- Web: `web_app_markdown.js` renders the report: headings, paragraphs, lists, quotes, fenced code, tables, code spans, emphasis, and http(s) links, escaping everything else. The report modal shows it rendered by default, with `Show Markdown` for the source; Copy still copies the Markdown. No dependency was added.
- `--report`'s description, `docs/INSPECT.md`, the Web report contract, and `internal/report/doc.go` describe the report as the run's record instead of an AI-ready report.

**Reason:** the user found notes alone unreadable for people and asked for one report rather than a second one; decisions 1-4 and the first log line are in [plan.md](plan.md), Phase 3. The user asked to implement it to see the UI.

**Plan impact:** Phase 3's first cut. Long first lines are cut at 160 characters and the table scrolls sideways in the modal; whether that is readable is for the user to judge.

**Validation:** new `TestRunReportRecordsSourcesNotesAndJobTable` (varying and shared variables, a variable set on one job only, notes on the run, the current attempt, and an older attempt, logs split over stdout and stderr, a pipe and backticks, a long line, a job without a log), `TestReportTableCodeQuotesAnyText`, `TestRenderMarkdownRendersReportsAndEscapesText` (raw HTML and `javascript:` links stay text), and the rendered-view checks added to `TestStaticWebReportRedactionToggle`; screenshots of trial 2's run in headless Chrome; `go test ./conformance/...` and `scripts/check.sh` with the race detector passed.

**Remaining:** the user's review of the rendered report; the job Notes modal still shows notes as plain text.
