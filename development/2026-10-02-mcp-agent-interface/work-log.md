# Work History: Agent-Facing MCP Interface

See [plan.md](plan.md) for current scope and status. Historical one-line notes did not preserve test results unless explicitly stated below; test files in diffs are not assumed to have passed.

## Explore the interface and prototype read-only inspection

**Commits:** 2026-10-02 13:09:21 `89241bb`; 2026-10-02 13:21:21 `f726a2b`; 2026-10-02 13:36:39 `54c18ae`; 2026-10-02 14:11:05 `9c6e5e8`.

**Change:** Added/reframed the agent-facing plan, explored master-directory-scoped discovery, and built an initial read-only MCP job-inspection prototype with command/package/docs/test changes.

**Reason:** Explore a typed agent interface with discovery limited to registered state.

**Plan impact:** Established the initial MCP direction and prototype; the agent trial later refined these assumptions.

**Validation:** Original one-line notes do not record test commands or results.

**Remaining:** Treat the initial raw-target model and command structure as superseded where the current plan differs; M0 remains next.

## Share inspection behavior and organize MCP commands

**Commits:** 2026-10-02 15:48:59 `486c79d`; 2026-10-02 15:55:08 `6a428aa`.

**Change:** Shared job inspection between MCP and terminal-facing code, grouped MCP server/agent entry points under `cmd/mcp`, and updated architecture/MCP docs.

**Reason:** Avoid duplicate behavior and clarify command/package ownership.

**Plan impact:** Advanced the thin-adapter direction and reorganized entry points; subsequent planning retired the duplicate agent command.

**Validation:** Historical notes do not preserve executed test commands or results.

**Remaining:** Continue with M0 and shared capabilities; do not assume the prototype command layout is final.

## Record the agent trial and reprioritize milestones

**Commits:** 2026-10-02 17:34:55 `e0d153a`; 2026-10-02 21:43:16 `fd4f635`; 2026-10-02 21:43:16 `aaf2234`.

**Change:** Explored a query/projection model, added a reproducible CLI agent-trial note and fixture, and rewrote the plan around observed information gaps. The current plan prioritizes compact shared summaries, treats CLI `--json` as the shell-agent interface, and keeps MCP a thin adapter.

**Reason:** The trial found that the CLI supports the basic fix loop; output volume and missing decision-ready summaries were the primary gaps.

**Plan impact:** Replaced generic query/projection with shared, task-shaped capabilities and M0-M7 milestones; recorded four CLI issues in `ISSUES.md`.

**Validation:** The fixture ran from scratch and reproduced 7/15 failures in `labA` and 84/300 in `labC`. The trial records calls, exit statuses, and output sizes. No Go tests ran for the fixture/docs change; `shellcheck` was unavailable. The plan rewrite was documentation-only and its note says referenced packages/commands were checked against the tree.

**Remaining:** Repeat the trial after each milestone; test an MCP-only client when read-only MCP tools exist. M0 is next.
