# Plan: Read-Side Performance on NFS

**Created:** 2026-10-15
**Status:** Done. Phases 0–4 and 6 done; Phase 5 not needed after Phase 4.
See [work-log.md](work-log.md).

## Purpose

On NFS, read commands such as `rotari jobs`, `info`, `show`, and `lineage` are
slow. Each `open`, `stat`, and directory read costs at least one round trip to
the server, because NFS close-to-open consistency revalidates attributes on
every open. These commands read files one at a time, read the same file many
times, and read every run in a project even when only a few are shown. Their
wall time therefore grows with the number of round trips, not with the amount
of data.

This plan makes each read command touch each state file at most once per
invocation, reads independent files concurrently, and skips files the output
does not need, without changing any output or contract.

## Non-goals

- Caching state across invocations, or in a long-lived process across polls.
  CORE-1 makes the filesystem the source of truth, and a supervisor or a
  compute node may write any attempt file at any time.
- Changing the status fallback chain (DUR-5) or what any view shows.
- Recommending NFS mount options (`actimeo`, `nocto`, `lookupcache`). They
  trade away the cross-host freshness rotari depends on.
- Speeding up writers (`run`, `retry`, the supervisor, wrappers).
- New CLI options. A tuning knob is considered only if Phase 3 measurements
  show that a fixed limit does not work (open question Q2).

## Baseline

Measured on 2026-10-15 with `strace -f -e trace=openat,newfstatat,getdents64`
on an NFS `TMPDIR`. The fixture had two projects with 15 runs each. Each
project's first run executed a 20-task array and one job. Each later run added
one job, carried the earlier results forward, and executed only the new job.
`open` counts include `ENOENT`.

| Command | open | getdents64 | Scales with |
|---|---|---|---|
| `basedirs` | 13 | 4 | constant |
| `projects` | 27 | 10 | projects |
| `runs` | 81 | 10 | runs (about 2.7 opens per run) |
| `gc --dry-run` / `wait --all` | 44 / 18 | 6 / 4 | constant |
| `info` | 110 | 12 | every run |
| `lineage -p P` (list) | 328 | 144 | every run, and every executed job in history |
| `lineage A B` (compare) | 139 | 88 | jobs of both runs |
| `show -p P` (latest run, mostly carried) | 85 | 28 | executed jobs |
| `show -r R` (21 executed jobs) | 583 | 384 | **about 28 opens per job** |
| `jobs` (all 30 runs in the window) | 899 | 570 | runs plus listed jobs |
| `jobs --since 0` | 168 | 10 | every run (about 5.6 opens per run) |

Repeated reads in that measurement:

- `show -r R`, per job:
  - `attempts/` listed 9 times;
  - `command.json` read 4 times;
  - `status.json` read 3 times, and the attempt directory stat'ed 3 times;
  - `submitted_at`, `finished_at`, `status`, and `scheduler_status.json`
    read twice each.

  Per run, `commands.json` is read 8 times, `summary.json` 7 times, and
  `running.lock` 5 times.
- `jobs`, per run, including runs outside the window:
  - `summary.json` read twice in full ([`CheckRunVersions`](../../internal/state/run_files.go)
    and then `LoadRunSummary` in
    [`collectRun`](../../internal/joblist/joblist.go));
  - `commands.json` and `carried.json` read;
  - the project's run lock read once per run ([`project.RunActive`](../../internal/project/inspect.go)).

  Per listed job, `attempts/` is listed 4 times
  ([`LatestAttemptJobDir`](../../internal/state/attempts.go),
  `LatestAttemptID`, and twice through `ReadJobTimestamp` in
  [`jobstatus.Timestamps`](../../internal/jobstatus/times.go)).
- `info`: reads every run's `summary.json`, and reads `running.lock` and
  `meta.json` once per run instead of once per project.
- `lineage` list: [`runview.LoadRun`](../../internal/runview/run.go) runs
  for every run. Per run it reads `commands.json` twice and `summary.json`
  twice, plus `carried.json`, `sources.json`, `notes.jsonl`, `context.json`,
  and `load_samples.jsonl`. It also resolves every job through the attempt
  files.

`--since` filters only the output. `jobs --since 0` still opens about five
files per run in the project's whole history.

## Current implementation

- Read paths:
  - [`showRun`](../../cmd/rotari/show.go) (run view; JSON and report share
    its helpers);
  - [`joblist.Collect`](../../internal/joblist/joblist.go) (`jobs`);
  - [`infoRuns`](../../cmd/rotari/info.go) (`info`);
  - [`runview.LoadRun`](../../internal/runview/run.go) and
    [`runview.RunsByStart`](../../internal/runview/order.go) (`lineage`, and
    other users of `runview`);
  - [`web.LoadRunJobs`](../../internal/web/loader.go) (Web API; MCP uses the
    same packages).
- Each caller resolves a job through several independent helpers, and each
  helper reopens files the others already read:
  - `state.LatestAttemptJobDir` and `state.LatestAttemptID` each list
    `attempts/`.
  - `jobstatus.ReadJob` / `ReadAttempt` read `status`, `status.json`, and
    `scheduler_status.json`.
  - `jobstatus.Timestamps` lists `attempts/` again and reads `submitted_at`
    and `finished_at`, falling back to `job.json` and `status.json`.
  - `jobstatus.FilterJob` locates the result attempt again and rereads
    `status.json` and the timestamps. `showRun` calls it for every job
    through `selectsShownJob`, even without a filter.
  - `state.ReadJobName`, `state.ReadAttemptExecutor`, and `readJSONCommand`
    read `name` and `command.json`.
- At run level, `state.CheckRunVersions` reads `commands.json`,
  `summary.json`, and `carried.json` only to check versions. Callers then
  load the same files again, and `jobstatus.RecordedResults` reads
  `carried.json` again.
- All reads are sequential.
- `state.Store` is a concrete struct with `ReadJSON` and `WriteJSON`. No read
  path holds the state lock.
- Long-lived readers poll: `web` (browser clients poll `/api/state`),
  `show --follow`, `wait`, and MCP. Each poll must see new writes.

## Decisions

- **D1 Snapshot scope.** A loaded snapshot lives for one command invocation,
  one Web request, or one poll iteration, and is then discarded. A file is
  read at most once inside that scope. Nothing is cached across scopes.
- **D2 One reader per level, owned by the existing packages.**
  - Run level: `state` gains one loader that reads `commands.json`,
    `summary.json`, and `carried.json` once, with the version check, and
    returns them together. It replaces the `CheckRunVersions` +
    `LoadQueue` + `LoadRunSummary` + `RecordedResults` sequence in every
    caller.
  - Attempt location: one call lists `attempts/` once and returns the latest
    attempt ID and directory together.
  - Attempt facts: `jobstatus` gains one reader that reads an attempt
    directory's files once into a value: status files, timestamps, name,
    recorded executor, `final_result.json`, and output modification times.
    `ReadJob`, `Timestamps`, `MeasureRunTime`, `AttemptAttributes`, and
    `FilterJob` evaluate that value instead of reopening files. The fallback
    chain stays in `jobstatus`, and callers do not read attempt files
    themselves.

  The old per-file helpers become unexported, or are removed once they have
  no caller, so a new caller cannot bypass the readers ("one rule, one
  implementation").
- **D3 Lazy reads for optional facts.** A fact that only some outputs need is
  read only when needed. Examples: `FilterJob` attributes when no filter
  condition reads them, log contents for diagnosis selectors, and output
  modification times for jobs that are not running.
- **D4 Bounded concurrency.** Independent jobs of a run, and independent runs
  of a listing, are read by a fixed-size worker pool (initially 16 workers).
  Results go into slots indexed by input position, so output order and the
  error reported stay deterministic: the first error in input order wins.
  Writes to shared state, such as stale lock removal, stay sequential.
- **D5 No behavior change.** `conformance/` must pass without edits. Every
  phase is a refactoring checked against the same contracts.
- **D6 Measured, not assumed.** Each phase records the baseline table
  re-measured on the same fixture. A phase that does not reduce round trips
  or wall time is reverted rather than kept for its structure alone.
- **D7 Read-count guard in tests.** A Linux test runs the built binary under
  `strace` on a small fixture and fails when a command exceeds a per-run or
  per-job bound, for example two listings of `attempts/` per job. It counts
  only paths inside the fixture, so Go runtime and system files do not
  matter, and it is skipped when `strace` is absent. CI installs `strace` in
  the Go job. Bounds are limits, not exact counts, so that unrelated reads
  added later do not churn the test; each phase tightens the bounds it
  achieves.
- **D8 Stale lock removal stays sequential, once per project.** Every reader
  removes a stale local run lock through `project.Inspect` /
  `InspectRunLock` (SAFE-11): after the consistency checks, under a shared
  state lock taken without waiting. Phase 1 reads the lock once per project,
  before the project's runs are read, and the removal happens there; the
  concurrent reads that follow see only that result.

## Phases

### Phase 0: Measurement harness

Add a script under `scripts/` that:

1. creates the baseline fixture under `$TMPDIR`, with `XDG_STATE_HOME` and
   `ROTARI_MASTERDIR` inside the work directory;
2. counts `openat`, `newfstatat`, and `getdents64` per command with `strace`,
   and records wall time;
3. removes the work directory.

The script records the counts but does not fail on them. It is for local
measurement, not CI. Commit the baseline output in `work-log.md`.

Also add the read-count test (D7) with bounds that the current code meets,
and add `strace` to the CI Go job's `apt-get install` line.

### Phase 1: Run-level files read once

- Add the run loader (D2) to `internal/state`. Use it from `showRun`,
  `joblist.collectRun`, `runview.LoadRun`, `web.LoadRunJobs` / `LoadJobs`,
  and `info`.
- Change `jobstatus.RecordedResults` to take the loaded summary and carried
  results instead of reading `carried.json` itself.
- In `jobs` and `info`, read the project run lock and `meta.json` once per
  project, and pass them to the per-run checks (`RunActive`, `RunPhaseOf`).
- Target: `jobs --since 0` opens no more than 2 files per finished run
  outside the window, down from 5.6.

### Phase 2: Attempt-level facts read once

- Add the single attempt-location call, and the attempt-facts reader in
  `jobstatus` (D2).
- Convert, one at a time with tests between them: `showRun` (table, JSON, and
  report), `joblist`, `runview.LoadRun`, `web.LoadJobs` / `loadAttempts`,
  and `info`.
- Make `selectsShownJob` / `FilterJob` reuse the facts already read for the
  job. For a carried job, they read only the origin attempt (D3).
- Target: `show -r R` lists `attempts/` once per job, and opens each attempt
  file at most once per job, down from about 28 opens per job.

### Phase 3: Concurrent reads

- Add the bounded pool (D4). Prefer a small helper next to its first users
  over a new general package, unless Phases 1 and 2 show a second package
  needs it.
- Apply it to the jobs of a run in `showRun`, `runview.LoadRun`, and
  `web.LoadJobs`; to the runs of a project in `joblist.Collect`,
  `runview.RunsByStart`, the `lineage` list, and `info`; and to projects
  across basedirs in `jobs`, `runs`, and `projects`.
- Run under the race detector (`scripts/check.sh`).
- Measure wall time on NFS with 1, 4, 16, and 32 workers before fixing the
  default.

### Phase 4: Skip unneeded files

- `jobs` and `runs`: decide whether a finished run is outside the window from
  the summary head (`state.LoadRunSummaryHead`). Load the full summary only
  for runs that are shown.
- `lineage` list: find out which per-job facts the list prints. If it prints
  only counts and failure summaries, resolve jobs only where the output needs
  them, through the same `jobstatus` evaluation.
- `info`: find out whether active and interrupted runs can be found from the
  project lock and `meta.json` without reading every run's summary. Keep the
  current scan where the metadata alone cannot identify an interrupted run.

### Phase 5: Optional — finished-run index

Do this only if Phases 1–4 leave `jobs`, `runs`, or `lineage` dominated by the
per-run cost of long histories.

The idea is a per-project index of finished runs and their finish times.
Readers would use it to skip old runs without opening their files. Writers
would update it under the state lock when they write `summary.json`, and it
must be rebuildable from run directories (CORE-1).

This needs a contract note and conformance rows, so it is a separate decision
recorded here before any implementation.

### Phase 6: Documentation

- `docs/OPERATIONS.md`: state that `-b` / `-p` narrow the scan, and what
  `--since` still reads for runs outside the window.
- `docs/ARCHITECTURE.md`: describe the run loader and the attempt-facts
  reader as the read path for run and job views.
- `contracts/README.md`: update only if Phase 5 adds a contract.

## Variants to check in every phase

- Job kinds: plain job, array task, whole array (`--partial-array`), matrix
  member.
- Views: queue view and run view; latest and older attempt; executed and
  carried result, including a carry chain.
- Run states: active, interrupted, and finished runs; runs with
  `carried.json` only, with `summary.json`, and with neither.
- Interfaces: CLI table, `--json`, `--report`, Web API, MCP, and the Python
  client.
- Long-lived readers (`web` polling, `show --follow`, `wait`): a write made
  between two polls is visible on the next poll (D1).

## Validation

- Before each conversion, add or extend table tests that give every variant
  above the same fixture and compare the output with the output before the
  change. The fixtures need distinct exit codes, hosts on some tasks only,
  carried and executed results, and more than one attempt.
- `go test ./internal/state ./internal/jobstatus ./internal/joblist
  ./internal/runview ./internal/web ./cmd/rotari` per step, then
  `go test ./conformance` unchanged, then `scripts/check.sh`.
- Re-run the Phase 0 script after each phase. Record the counts and the NFS
  wall time in `work-log.md`.

## Open questions

None. Q1 (read-count guard) became D7, Q3 (stale lock removal under
concurrent reads) became D8, and Q2 (worker count) was decided by the Phase 3
measurements: a fixed limit of 16, without an environment variable (see
[work-log.md](work-log.md)).
