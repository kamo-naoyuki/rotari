# Work History: Unified Notifications

See [plan.md](plan.md) for current scope and status. The original one-line notes did not preserve test commands/results; test changes are not reported as passing runs.

## Settle the shared notification design and settings

**Commits:** 2026-10-01 00:15:27 `d44159b`; 2026-10-01 00:23:08 `f3a367b`; 2026-10-01 00:25:03 `e9607e1`; 2026-10-01 01:43:47 `033cabe`.

**Change:** Created/refined the notification plan and aligned webhook/browser settings on `notifications.toml` across CLI/configuration, run setup, Web, contracts, and tests.

**Reason:** Replace independent channel configuration models with common fields and validation while retaining per-channel values and secret handling.

**Plan impact:** Settled the settings direction and advanced settings-resolution/Webhook work.

**Validation:** Commits include configuration and webhook tests; historical notes do not retain commands or results.

**Remaining:** Verify precedence, authorization, and secret-safety requirements in the current implementation.

## Apply configured payload fields and consolidate guides

**Commits:** 2026-10-01 03:25:19 `a5d0e5c`; 2026-10-01 03:33:32 `9737f8c`.

**Change:** Applied configured fields to event/payload projections and browser notification UI, updated tests/contracts, and consolidated user guides into `docs/NOTIFICATIONS.md` with navigation/reference updates.

**Reason:** Make configured fields effective throughout delivery and give users a coherent guide.

**Plan impact:** Advanced configured delivery and documentation consolidation.

**Validation:** Tests/docs changed; old notes do not specify which checks ran or passed.

**Remaining:** Review delivery failure, batching, browser reload, and secret handling against the plan.

## Fix run snapshots and Web settings surfaces

**Commits:** 2026-10-01 21:45:55 `8a12006`; 2026-10-01 21:45:55 `b6bfe65`; 2026-10-01 21:45:55 `faad558`.

**Change:** Limited run snapshots to loaded configuration, fixed basedir-aware settings resolution, and rendered notification settings in the Web configuration modal, with related tests and runtime changes.

**Reason:** Keep run configuration explicit and present the same basedir-aware settings in Web UI.

**Plan impact:** Advanced runtime snapshot correctness and Web settings UX.

**Validation:** Commits add tests, but historical notes do not record execution results.

**Remaining:** Check atomic reload and secret-safe Web/API behavior against the plan.
