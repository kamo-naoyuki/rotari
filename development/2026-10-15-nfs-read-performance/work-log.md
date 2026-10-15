# Work log: Read-Side Performance on NFS

## 2026-10-15 12:06:57 +0900 — `b8f43472`: Phase 0 measurement harness

- **Change:** added [scripts/measure-state-reads.sh](../../scripts/measure-state-reads.sh),
  which builds a fixture under `$TMPDIR` and reports each read command's wall
  time and opens, directory listings, stats, and `getdents64` calls; and
  `TestReadCountsStayBounded` in
  [conformance/read_count_test.go](../../conformance/read_count_test.go),
  which runs `show` (an executed run and the latest run), `jobs`,
  `jobs --since 0`, `runs`, `info`, and `lineage` under `strace` and bounds
  how often each kind of state file is touched per path. The CI Go job
  installs `strace`.
- **Reason:** D6 and D7: every later phase is measured against this
  baseline, and the bounds catch a phase that adds reads back.
- **Plan impact:** Phase 0 done. The bounds are today's maxima, over a
  default of two touches per file. Phase 1 and 2 lower them.
- **Validation:** the read-count test passed six consecutive runs after
  the executed run was chosen by run ID rather than by directory order (run
  IDs from the same second sort by their random suffix, which made one run
  out of three fail). `go test ./conformance` passed. The script's
  baseline on this host's NFS `$TMPDIR` (2 projects × 15 runs, 20-task
  array; wall times are single cold runs and noisy):

  | command | seconds | open | list | stat | dents |
  |---|---|---|---|---|---|
  | `basedirs` | 0.013 | 14 | 2 | 9 | 4 |
  | `projects` | 0.194 | 58 | 11 | 12 | 23 |
  | `runs` | 0.172 | 194 | 11 | 90 | 22 |
  | `runs --since 0` | 0.215 | 194 | 11 | 90 | 22 |
  | `info -p p1` | 0.031 | 108 | 3 | 30 | 6 |
  | `lineage -p p1` | 0.118 | 329 | 72 | 63 | 144 |
  | `show -p p1` | 0.028 | 87 | 16 | 32 | 28 |
  | `show -p p1 -r FIRST` | 0.143 | 584 | 194 | 93 | 384 |
  | `jobs` | 0.885 | 1177 | 303 | 88 | 606 |
  | `jobs --since 0` | 1.582 | 447 | 23 | 18 | 46 |

  Per-file maxima from the test fixture include, for `show` of an executed
  run: `attempts/` listed 9 times, `command.json` opened 4 times, the run's
  `commands.json` 8 times and `summary.json` 7 times.
- **Remaining:** Phases 1–6; open question Q2.

## 2026-10-15 13:11:08 +0900 — `9eb0d49d`, 13:15:01 +0900 — `661967f3`: Phase 1 run-level files

- **Change:** `state.LoadRunSnapshot` reads a run's `commands.json`,
  `summary.json`, and `carried.json` once, refusing a newer state version
  as `CheckRunVersions` did (which now calls it). `jobstatus.RecordedResults`
  takes the snapshot instead of rereading `carried.json`. `jobs`
  (`joblist.collectRun`), `show` (table, `--json`, job JSON, job result
  lookup), `runview.LoadRun` (`lineage` and others), `web.LoadRunJobs`
  (Web API, report, bug report, history search), `info`, job control, and
  rerun planning use it. `show`'s run table also stopped rereading
  `commands.json` for the queue comparison, the job specs, and the scope;
  `compareQueueWithRun` takes loaded queues. `web.LoadRunJobs` lost its
  summary parameter and reads the summary from the run, as every other view
  does. In the second commit, `jobs` reads the project's run lock once per
  project instead of once per run, and `info` inspects the project once
  and no longer reads every run's summary: only the project's running or
  interrupted run can be either.
- **Reason:** Phase 1, D2 (run level) and D8.
- **Plan impact:** Phase 1 done except its `jobs --since 0` target of two
  opens per finished run outside the window: it is three (`commands.json`,
  `summary.json`, `carried.json`); Phase 4's summary-head check reaches
  two. Read-count bounds lowered: `show` of an executed run now opens
  `commands.json` at most 3 times per file (from 8) and `summary.json` 5
  (from 7); the per-invocation remainder comes from project inspection and
  lifecycle labels, not from jobs.
- **Validation:** unit tests of the touched packages passed;
  `internal/webui` and `cmd/rotari` showed only the 32 and 2 Node failures
  this host always has (same test names as before the change). Full
  `go test ./conformance/...` for the first commit failed only
  `TestShowReportsElapsedAndQuietTime` (NFS clock skew, ISSUES.md) and
  `TestStaticExportPagesShowWhatLivePagesShow` (old Node). For the second,
  `go test ./conformance -run TestReadCountsStayBounded` (three runs),
  `./conformance/03-interfaces -run 'Info|Jobs|NotStarted'`, and
  `./conformance/04-coordination` passed. Script counts on NFS (opens):
  `info -p p1` 108 → 22, `jobs` 1177 → 996, `jobs --since 0` 447 → 296,
  `lineage -p p1` 329 → 299, `show -p p1 -r FIRST` 584 → 532; wall times
  were too noisy on this shared NFS to compare.
- **Remaining:** Phases 2–6.
