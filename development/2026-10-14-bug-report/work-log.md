# Work Log: Bug Report Bundle

See [plan.md](plan.md).

## Phase 1: Scheduler query outcome log

- `5b4cb537` (2026-10-14 15:18:00 +0900)

**Change:** `internal/executor` writes `scheduler_events.jsonl` in each
scheduler job attempt directory (`scheduler_events.go`). `waitForSchedulerResult`
and `resolveMissingSchedulerState` record queue and accounting outcomes when
they change, and the result's source (`wrapper`, `accounting`,
`unavailable`). `schedulerQuery` and `schedulerAccounting` carry `Err` in
place of `Failed`, and the Slurm, PBS, SGE, and LSF queue and accounting
functions keep the command output in their errors through
`SchedulerCommandHint`. `state.ValidatedStateFile` accepts the new file.
Contract note beside `scheduler_status.json` in
`contracts/02-run-lifecycle-and-execution.md`. The plan was added in the
same commit.

**Reason:** the 2026-10-14 Slurm report (a running job recorded as finished
with error `running`) needed the times when squeue failed and recovered,
which rotari did not keep. A bundle can only collect evidence that exists.

**Plan impact:** Phase 1 done. D1–D3 held; the `accounting_wait` events
first written were dropped, since they repeat on every poll while sacct
reports the job running.

**Validation:** `go test ./internal/executor` focused run (new
`TestSchedulerEvents*`, `TestSlurmQueryErrorsKeepCommandOutput`, and the
existing wait tests) passed; `go vet ./...` passed; `go test -short ./...`
failed only `TestShowReportsElapsedAndQuietTime` in
`conformance/03-interfaces`, which reports "quiet 14m" from the NFS clock
offset of `$TMPDIR` on this host and is unrelated; `go test -race
./internal/executor ./internal/state` passed.

**Remaining:** Phases 2–4.

## Phase 2: `rotari bug-report`

- `6237eb965` (2026-10-14 15:41:04 +0900)

**Change:** new `internal/bugreport` (`Collect`, redaction, `WriteTarGz`,
filesystem type and clock offset of the basedir) and
`cmd/rotari/bug_report.go`; `cmdInfo`'s report moved into
`buildInfoReport` for reuse. `internal/executor` gained
`ProbeSchedulerJob` and per-scheduler queue and accounting command
builders, which the polling functions now use too. `cli_spec.go` entry,
regenerated `docs/CLI_REFERENCE.md` and `python/rotari/generated_cli.py`,
updated CLI schema golden files, the flag-pair inventory entry (deferred,
documented in `docs/internal/testing/cli-flag-pair-coverage.md`), and
`docs/ARCHITECTURE.md`.

**Reason:** plan Phase 2 (D4–D10).

**Plan impact:** `--job` became `--job-id` and accepts attempt IDs, whose
run it selects. Environment values rotari sets itself (`ROTARI_*`) are kept,
since they are its own state; all other environment values are replaced.
Paths become `[BASEDIR]`, `[HOME]`, `[CWD]` placeholders before the generic
patterns, so the structure stays readable. The run's `show --report` text
is included. At most 10 jobs are probed live.

**Validation:** `go test ./internal/bugreport` (secrets absent with and
without redaction, path redaction, job selection and output modes, live
Slurm queries through fake commands, the environment-only report leaving
nothing in the basedir, the tar writer) passed; a manual run in a temporary
basedir with a webhook URL, auth token, and job secret found none in the
summary or the extracted bundle. `go vet ./...` passed; `go test -short
./...` failed `TestCLIFlagPairInventory` (fixed in this commit, then passed
alone) and the unrelated `TestShowReportsElapsedAndQuietTime`; `go test
./conformance ./internal/doclinks ./internal/archtest` and `go test -race
./internal/bugreport ./internal/executor` passed; Python tests passed.

**Remaining:** Phases 3–4.

## Phase 3: Contract, conformance, FAQ

- `690784563` (2026-10-14 15:47:38 +0900)

**Change:** CLI-26 in `contracts/03-server-and-command-interfaces.md` and
its status row; `TestBugReportNeverIncludesSecrets` in
`conformance/03-interfaces/bug_report_test.go`, which reads every file of
the bundle; a FAQ entry on reporting a bug. The same commit records the
Phase 4 findings in the plan.

**Reason:** plan Phase 3; the secret guarantee is the promise users rely on
before sharing a bundle.

**Plan impact:** Phase 3 done. OPERATIONS and RECOVERING were not changed.

**Validation:** `go test ./conformance/03-interfaces -run
'TestBugReportNeverIncludesSecrets|TestCLIFlagPairInventory'`, `go test
./conformance ./internal/doclinks`, `scripts/sync_readme.py --check`, and
`generate_cli_reference.py --check` passed.

**Remaining:** Phase 4 step 4.

## Phase 4: Local check against the 2026-10-14 Slurm bug

No commit; findings are in [plan.md](plan.md#phase-4-check-the-bundle-against-the-slurm-bug-locally-steps-13-done).
The bundle alone showed both causes: sacct's `running` taken as a result,
and the accounting wait not restarting after the queue recovered.
