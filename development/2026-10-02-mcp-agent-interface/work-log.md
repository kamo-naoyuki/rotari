# Work History: Agent-Facing MCP Interface

See [plan.md](plan.md) for current scope and status. Historical one-line notes did not preserve test results unless explicitly stated below; test files in diffs are not assumed to have passed.

## Explore the interface and prototype read-only inspection

**Commits:** `820acb4`; `d8a140f`; `d30dc7a`; `d8de696`.

**Change:** Added/reframed the agent-facing plan, explored master-directory-scoped discovery, and built an initial read-only MCP job-inspection prototype with command/package/docs/test changes.

**Reason:** Explore a typed agent interface with discovery limited to registered state.

**Plan impact:** Established the initial MCP direction and prototype; the agent trial later refined these assumptions.

**Validation:** Original one-line notes do not record test commands or results.

**Remaining:** Treat the initial raw-target model and command structure as superseded where the current plan differs; M0 remains next.

## Share inspection behavior and organize MCP commands

**Commits:** `854261d`; `35b7ba4`.

**Change:** Shared job inspection between MCP and terminal-facing code, grouped MCP server/agent entry points under `cmd/mcp`, and updated architecture/MCP docs.

**Reason:** Avoid duplicate behavior and clarify command/package ownership.

**Plan impact:** Advanced the thin-adapter direction and reorganized entry points; subsequent planning retired the duplicate agent command.

**Validation:** Historical notes do not preserve executed test commands or results.

**Remaining:** Continue with M0 and shared capabilities; do not assume the prototype command layout is final.

## Record the agent trial and reprioritize milestones

**Commits:** `167b6ee`; `c12fb5d`; `bc22864`.

**Change:** Explored a query/projection model, added a reproducible CLI agent-trial note and fixture, and rewrote the plan around observed information gaps. The current plan prioritizes compact shared summaries, treats CLI `--json` as the shell-agent interface, and keeps MCP a thin adapter.

**Reason:** The trial found that the CLI supports the basic fix loop; output volume and missing decision-ready summaries were the primary gaps.

**Plan impact:** Replaced generic query/projection with shared, task-shaped capabilities and M0-M7 milestones; recorded four CLI issues in `ISSUES.md`.

**Validation:** The fixture ran from scratch and reproduced 7/15 failures in `labA` and 84/300 in `labC`. The trial records calls, exit statuses, and output sizes. No Go tests ran for the fixture/docs change; `shellcheck` was unavailable. The plan rewrite was documentation-only and its note says referenced packages/commands were checked against the tree.

**Remaining:** Repeat the trial after each milestone; test an MCP-only client when read-only MCP tools exist. M0 is next.

## M0: Retire `rotari-agent` and locate MCP runs through the master registry

**Commits:** 2026-10-02 22:10:34 `dac2691`.

**Change:** Removed `cmd/mcp/agent` (`rotari-agent`) and its test. `rotari_get_job_info` in `internal/mcp` now takes only `run_id` and `job_id`; `NewServer` takes the master directory, and the tool finds the run's basedir and project through that directory's run registry. Added `resolve.RegisteredRun`, which has no fallback to a default or working-directory basedir, and moved the stale-run check into `requireRunDirectory`, shared with `resolve.ExistingRun`. `cmd/mcp/server` resolves the master directory with `state.ResolveMasterDir` and exits non-zero when it cannot start or fails. Updated `docs/MCP.md`, `docs/ARCHITECTURE.md`, and `plan.md`.

**Reason:** M0 of the plan. The prototype took a raw basedir path from the agent, `rotari-agent` duplicated `rotari show --report` and depended on the MCP package, and the server exited 0 on failure.

**Plan impact:** M0 is done. The "MCP configuration" decision now names the run registry as the only route to a run. Structured result fields beyond identity were deferred to M4, where they will come from the M1-M3 shared capabilities instead of a projection inside the adapter. M1 (failure grouping) is next.

**Validation:**
- New `TestRegisteredRunHasNoFallback` (`internal/resolve`) covers: a run in another master directory, a stale entry, and `/` and `\` in run IDs.
- Rewritten `internal/mcp` tests cover two same-named projects in different basedirs over MCP, an input schema without location fields, and unregistered, path-unsafe, and unknown run and job IDs.
- `go test ./internal/resolve ./internal/mcp ./internal/archtest` passed. `go test ./conformance/...` passed in all seven packages.
- `scripts/check.sh --short` and `scripts/check.sh` failed only in `TestWebJobsPageShowsRecentJobs`, which also fails on the parent commit and is already in `ISSUES.md`.
- The built `rotari-mcp` was run over stdio against the trial fixture. It returned the timed-out `labA` task's report from `run_id` and `job_id` alone, reported an unregistered run as a tool error, and exited 1 when no master directory could be resolved.
- `pre-commit` is not installed here and was not run; `gofmt -l` and `go vet` reported nothing.

**Remaining:** M1, failure grouping. `TestWebJobsPageShowsRecentJobs` remains open in `ISSUES.md`.
