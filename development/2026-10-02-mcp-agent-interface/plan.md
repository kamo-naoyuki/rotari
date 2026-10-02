# Plan: Agent-Facing MCP Interface

**Created:** 2026-10-02

## Purpose

Explore how an agent can discover and inspect rotari state without being given filesystem paths or having to reproduce human-oriented screen flows. This is a design/implementation exploration, not a final public API specification. Keep read-only inspection separate from queue mutation and execution; do not grant write/run capabilities implicitly.

## Discovery model

An MCP server is configured for one master directory. Its registry may contain multiple basedirs, so masterdir and basedir are not a 1:1 relationship. Expose a basedir-scoped reference and preserve that scope through project, run, job, attempt, and log lookup. Never confuse identical project names or IDs across basedirs. Avoid requiring agents to pass raw basedir/state paths.

A useful information hierarchy is:

```text
MCP server -> configured masterdir -> registered basedirs -> project -> run -> job/attempt/log/diagnosis
```

The first response should help an agent choose where to look; deeper queries should return only the requested projection. Candidate overview data includes basedir label/existence, project count, recent activity, active runs, and recent failures.

## Query and projection concept

Consider a shared query with explicit hierarchical scope (`basedirs`, `projects`, `run_ids`, `job_ids`), filters, included fields, and strict limits. Candidate projections include summaries, results, diagnosis, command/job definition, and bounded stdout/stderr excerpts. Results should carry all identifiers needed for a follow-up request, state the searched scope, and disclose truncation, unavailable data, or ambiguous candidates. Apply parent-child relationships rather than treating ID lists as a Cartesian product.

Reuse existing resolution, status, selection, and history-search semantics where possible; do not call the Web API blindly or implement a second set of rules. Evaluate whether search/projection logic can be shared across Web and MCP while keeping transport and presentation adapters separate.

## Information to evaluate

- Which basedir and project contain the relevant state?
- Which run is latest, active, failed, or comparable to a previous run?
- Which jobs failed, what were their dependencies and attempt histories, and what diagnosis applies?
- Which bounded log excerpts explain the failure?
- What changed between runs, and did retry improve the outcome?
- Should the agent only explain a possible retry, or may it preview/mutate/run after explicit human approval?

Do not return secrets, environment values, executor options, or sensitive paths by default. Treat commands and logs as potentially sensitive. Large logs/artifacts require byte/line limits and an explicit indication of truncation. Do not guess among multiple matching projects, runs, or jobs.

## MCP transport concepts

MCP protocol methods (`initialize`, `tools/list`, `tools/call`, `resources/list`, `resources/read`) are distinct from rotari tool names (for example, names advertised through `tools/list`). `capabilities` reports feature categories, not a mode switch. Decide among Tools, Resources, and Prompts only after the information and interaction model is tested; these mechanisms are not mutually exclusive.

## Current status and follow-ups

A read-only job-inspection prototype has been added, inspection logic is shared with terminal-facing code, and MCP entry points are grouped under `cmd/mcp`. The query/projection model remains exploratory. Re-evaluate the design against the implementation before committing to API names, schema, opaque references, paging, or cross-basedir behavior.

Next, exercise realistic requests against a master directory with multiple basedirs: discover state, summarize a failed run, compare runs, search bounded logs, and avoid collisions between same-named projects. Record what context the agent needs initially versus what should be fetched on demand. Decide separately about write/run operations, authorization, approval, timeouts, and idempotency.

## Open decisions

- One general query tool versus separate discovery and detail operations.
- Tools, Resources, Prompts, or a combination.
- Reversible basedir IDs versus opaque references and their lifecycle.
- Search scope, paging/default time range, and limits for logs and records.
- Which command/configuration details are safe and useful to return.
- Whether Web history-search scope and MCP projection can share a model.
- Whether any future mutation/execution support belongs in MCP and where approval is mandatory.
- Whether a dedicated VS Code extension is necessary.
