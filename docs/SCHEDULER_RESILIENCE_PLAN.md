# Scheduler submission and polling resilience plan

## Purpose

Rotari should avoid needlessly increasing load on scheduler controllers while
preserving the current serverless architecture. This is a common scheduler
feature for Slurm, PBS, and LSF; executor-specific command details remain
inside each executor.

## Current behavior

- `RunBatchLane` limits each non-local executor lane to its effective
  concurrency, then waits for that batch before submitting the next one.
- Each scheduler executor submits an ordinary job through its native command;
  compatible array tasks use one native array submission where supported.
- Scheduler executors poll their queue command at a fixed interval. Once a job
  is absent, they use their accounting command and the job wrapper's
  `status.json`, then give up after an accounting deadline when neither source
  supplies a terminal result.
- Explicit scheduler controller or transport submission failures retry at most
  twice after 1 and 2 seconds. Permanent errors and ambiguous outcomes fail
  immediately. Each scheduler has its own 100ms per-process submit interval,
  including native arrays and retry attempts. Repeated queue or accounting
  query failures back off from the normal poll interval by powers of two,
  capped at 30 seconds; a successful query restores the normal interval. The
  default jitter source preserves the calculated delay.

## Implementation checklist

- [x] Extract the shared scheduler wait lifecycle: wrapper status fallback,
  queue-state persistence, accounting deadline, and polling.
- [x] Add scheduler-focused regression tests for submission failure, queue-query
  failure, accounting-query failure, and no duplicate submission after an
  ambiguous outcome.
- [x] Introduce a shared timing policy with injectable clock, sleeper, and
  deterministic jitter source for tests.
- [x] Add bounded polling backoff after repeated queue or accounting query
  failures; reset it after a successful scheduler query.
- [x] Define transient submission failures per scheduler and reject permanent
  configuration, authorization, and destination errors without retrying.
- [x] Add bounded submission retry with jitter, preserving the no-duplicate
  guarantee for ambiguous submit outcomes.
- [x] Add per-process submission spacing. Decide whether the initial setting is
  shared by all scheduler executors or executor-specific.
- [x] Expose finalized controls through configuration, environment variables,
  CLI flags, generated config/schema output, and Web UI where applicable.
- [x] Document the resulting user-facing contract in README, FAQ, and internals.
- [ ] Run focused executor tests, `go test ./...`, and scheduler integration
  tests where a scheduler environment is available.

## Scope and decisions

- Add per-process controls for spacing scheduler submissions and retrying
  transient submission failures.
- Improve handling of repeated polling-command failures so an unavailable
  scheduler controller is not queried at a fixed high rate by every active job.
- Keep the controls scheduler-oriented and share mechanics in
  `internal/executor` where appropriate, while retaining native command and
  status semantics in each executor.
- Preserve the existing default behavior unless an explicit option or a
  deliberately documented default change is introduced.
- Do not coordinate limits across rotari processes, projects, hosts, or
  basedirs. Cluster-wide fairness and hard submission limits remain scheduler
  administration concerns, such as Slurm QoS and account limits.
- Do not add a coordinator, database, shared lock protocol, or communication
  between rotari servers for this feature.

## Design requirements

### Submission spacing

- A configured interval must delay only scheduler submissions. It must not
  delay local execution or completion/result persistence.
- Native array submission counts as one scheduler submission.
- The behavior must be deterministic in tests through an injected clock or
  sleeper; tests must not depend on real-duration waits.
- Decide and document whether the interval is common to all non-local
  executors or configured per executor. Prefer the existing executor-specific
  configuration pattern if both are needed.

### Transient submission retry

- Submit failures are classified before retrying: controller or transport
  unavailability is transient; permission, account, queue/partition, resource,
  and option errors are permanent; a timeout or successful command without a
  usable job ID is ambiguous.
- Retry only errors that plausibly represent temporary scheduler unavailability
  or transport failure. Invalid executor options, permissions, and invalid
  scheduler destinations or accounts must fail immediately.
- Use a bounded exponential delay with jitter and a configurable retry limit.
- Make each retry visible in progress logging, including the scheduler,
  attempt number, and next delay, without exposing command output that may
  contain sensitive data.
- Treat an ambiguous submission result carefully: a retry is safe only when
  the native submit command clearly failed before returning a job ID. A timeout
  after a scheduler may have accepted a submission must not blindly submit a
  duplicate job.

### Polling failure behavior

- Continue checking the wrapper's `status.json`, because it remains an
  independent terminal-result source when scheduler accounting is delayed or
  unavailable.
- Apply backoff only after scheduler query failures; reset to the normal poll
  interval after a successful query.
- Bound the delay so cancellation and completion visibility do not become
  unreasonably slow, and preserve the existing accounting deadline semantics
  unless separately changed.
- Avoid changing the successful-query polling cadence in the first iteration.

## Delivery sequence

1. Add focused tests that characterize each scheduler executor's native submit,
  queue-query, and accounting-query failure behavior, including the
  no-duplicate-submit condition. Start with Slurm as the representative
  implementation.
2. Introduce a small, testable timing/backoff policy with an injected clock or
   sleep function. Do not expose CLI/configuration yet.
3. Apply bounded polling backoff to the scheduler executors. Run the executor
  package tests.
4. Add submission retry classification and bounded backoff to scheduler
  executors, with explicit handling for ambiguous timeouts.
5. Expose the settled controls through existing config, environment, CLI, and
  generated web/config documentation.
6. Update README, FAQ, and internals documentation with the final user-visible
   contract and run the relevant package tests followed by `go test ./...`.

## Acceptance criteria

- Configured submission spacing prevents rapid consecutive scheduler submits
  from one rotari process.
- A known transient submit failure retries at most the configured number of
  times, using increasing bounded delays.
- A known permanent submit failure is attempted once.
- Ambiguous submit outcomes never cause an automatic duplicate submission.
- Repeated `squeue` or `sacct` failures increase the wait before later
  scheduler queries while wrapper status remains observable.
- Existing array batching, scheduler status persistence, cancellation, and
  successful job completion behavior remain covered by tests.
