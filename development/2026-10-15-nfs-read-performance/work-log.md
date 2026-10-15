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
