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
