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

## Intended functional coverage

The long-term goal is not only job inspection. The agent-facing interface should eventually cover the useful rotari capabilities across these domains:

| Domain | Candidate capabilities | Interaction shape |
| --- | --- | --- |
| Discovery and inspection (`show`, `jobs`) | Find basedirs/projects; inspect queue, run, job, attempt, result, diagnosis, and bounded logs | Common scoped query with filters and a requested projection |
| Readiness (`check`) | Validate whether the selected project/queue can run and return actionable issues | Read-only operation with structured findings; may be exposed as a query projection if that stays clear |
| Workflow (`export`, `import`) | Export a run/queue as a manifest; validate and preview a proposed import; apply the import | Export is a read operation; import is a state-changing operation with a separate preview and apply step |
| Queue operations | Add, change, remove, copy jobs; inspect the resulting queue | Explicit operations with a structured change plan and an apply step where useful |
| Run operations | Start/run, retry selected jobs, wait/inspect progress, cancel, suspend, resume | Explicit lifecycle operations; start asynchronously and return stable run identity |
| Project/history cleanup | Delete runs/jobs, garbage-collect history/state, reset a queue/project | Destructive operations, individually named and guarded; never hidden inside a generic query |

The query model is for selecting *what data to return* and is a natural fit for `show`, `jobs`, log search, and possibly `check`. It should not become a generic command interpreter for writes. Mutations and lifecycle transitions should have explicit operation names and typed arguments, so the agent can distinguish reading a proposed change from applying it. `export` is read-only but may return a large manifest; `import`, `delete`, and `reset` modify or discard state and need stronger safeguards.

## Unified object-oriented interaction (UI/API idea)

The same idea can be viewed as a database-like object browser/API rather than a collection of unrelated command replicas:

1. **Query objects** by object kind (`basedir`, `project`, `queue`, `run`, or `job`) and conditions. The query returns typed object records, each with a stable scoped identity and a requested information projection.
2. **Act on returned object identities** through explicit operations. For example: retrieve more information with selected fields; `check`; `export`; preview/apply `import`; `add`; `run` with run options; `delete`; or `reset`.
3. **Return the affected object(s) and outcome** in structured form so a follow-up query can continue from their identities.

Conceptually, a result might contain `type`, `identity` (basedir/project/run/job IDs as applicable), `data` (the requested projection), and perhaps `available_operations`. The exact shape is open. Exposing possible operations on an object can help an agent discover what applies, but the server must validate every requested action: not every operation applies to every object, and an advertised/hinted action is not authorization.

Keep the two parts distinct even if the client presents them as one interface: **query selects/reads objects; operation requests a state transition**. A `job` result could support more-info, log projections, and job control; a `queue` or `project` may support check/add/import/run/reset; a `run` may support inspect/export/retry/cancel/delete. These are candidate mappings, not a promise that each object gets every action. `reset`, `delete`, `import`, queue edits, and `run` need explicit target identity, typed options, precondition validation, and a preview/approval/apply flow appropriate to the operation. Never encode arbitrary shell commands or accept an untyped `operation: "<CLI command>"` escape hatch.

This could provide a consistent interaction for both `rotari-agent` and MCP: terminal input/output can be JSON, while MCP wraps the same query and operation services as tools. It does not imply a visual UI must be built first; “UI” here means the agent-facing object/action model.

The terminal entry point (`rotari-agent`) and MCP entry point should expose the same protocol-neutral operation set and structured inputs/outputs. Transport adapters may format/encode results differently (JSON on stdout versus MCP structured results), but must call the same service functions and preserve the same validation, selection, status resolution, and error semantics. Avoid implementing a second CLI-like behavior inside the MCP adapter.

Before calling the API complete, maintain a parity matrix from every candidate operation to its existing rotari implementation and contract tests. This should identify unsupported options/variants explicitly; an operation must not silently ignore a flag or selector.

## Sharing and package boundaries

Supporting these domains must **not** mean reimplementing all of `cmd/rotari` in `rotari-agent` and MCP. That would create a second CLI/API whose queue, selector, status, locking, and run behavior would drift from rotari itself.

- Keep `cmd/rotari` as the human-oriented CLI adapter. Keep `cmd/mcp/agent` and `cmd/mcp/server` as thin transport/argument/output adapters.
- Each behavior has one owner: use the existing shared packages for project resolution, status/result resolution, job selection, queue edits, workflow reconciliation, run lifecycle, job control, reporting, and history search. When a capability currently exists only inside a command handler, first extract the reusable operation to an appropriate lower-level package; do not copy the handler into an agent package.
- Keep protocol-neutral request/response and application operations separate from MCP SDK types. MCP should map typed MCP inputs to the shared operation and encode its result; `rotari-agent` should parse flags/JSON, call that same operation, and emit JSON. Neither adapter should call the other.
- Add one integration/parity test per shared behavior proving both entry points reach the same operation/result, plus package/conformance tests for the owning domain rules. Don't duplicate the domain test suite for every transport.
- The parity matrix is a coverage map, not a demand to expose every CLI flag. An agent operation should expose only coherent agent use cases and necessary selectors, while preserving the relevant underlying contract.

This may require a protocol-neutral internal service package (for example, a narrowly scoped `internal/agentapi`) alongside `internal/mcp`; exact package names are open. The key boundary is that reusable behavior and result types must not depend on MCP SDK request/result types or on `cmd/rotari`.

## Information and operations to evaluate

- Which basedir and project contain the relevant state?
- Which run is latest, active, failed, or comparable to a previous run?
- Which jobs failed, what were their dependencies and attempt histories, and what diagnosis applies?
- Which bounded log excerpts explain the failure?
- What changed between runs, and did retry improve the outcome?
- Should the agent only explain a possible retry, or may it preview/mutate/run after explicit human approval?
- Which human-facing commands map naturally to a scoped query, and which must remain explicit operations?
- Which object kinds should the query return (`basedir`, `project`, `queue`, `run`, `job`), and what stable identity does each need?
- Should results advertise applicable operations, or should the agent learn them from tool schemas only?
- For add/change/copy/import/delete/reset/retry/run/control operations, what is the preview, confirmation, apply, and result-reporting sequence?
- How do idempotency, retries after transport timeout, asynchronous run handles, cancellation, and partial failures work for state-changing calls?

Do not return secrets, environment values, executor options, or sensitive paths by default. Treat commands and logs as potentially sensitive. Large logs/artifacts require byte/line limits and an explicit indication of truncation. Do not guess among multiple matching projects, runs, or jobs.

## MCP transport concepts

MCP protocol methods (`initialize`, `tools/list`, `tools/call`, `resources/list`, `resources/read`) are distinct from rotari tool names (for example, names advertised through `tools/list`). `capabilities` reports feature categories, not a mode switch. Decide among Tools, Resources, and Prompts only after the information and interaction model is tested; these mechanisms are not mutually exclusive.

## Current status and follow-ups

A read-only job-inspection prototype has been added, inspection logic is shared with terminal-facing code, and MCP entry points are grouped under `cmd/mcp`. The query/projection model remains exploratory. Re-evaluate the design against the implementation before committing to API names, schema, opaque references, paging, or cross-basedir behavior.

Next, exercise realistic read queries against a master directory with multiple basedirs: discover state, summarize a failed run, compare runs, search bounded logs, and avoid collisions between same-named projects. Record what context the agent needs initially versus what should be fetched on demand. Then inventory the full desired command coverage and map each operation to shared rotari logic and contracts. Add state-changing operations only after their preview/apply boundary, authorization, approval, timeout, idempotency, and recovery behavior are designed.

## Open decisions

- One general query tool versus separate discovery and detail operations.
- Tools, Resources, Prompts, or a combination.
- Reversible basedir IDs versus opaque references and their lifecycle.
- Search scope, paging/default time range, and limits for logs and records.
- Which command/configuration details are safe and useful to return.
- Whether Web history-search scope and MCP projection can share a model.
- How the protocol-neutral shared operation layer is separated from MCP and terminal transport adapters.
- The complete command/operation parity matrix, including variants and contract coverage.
- Whether mutation/execution support is exposed to agents, and where preview, confirmation, authorization, and audit are mandatory.
- Whether a dedicated VS Code extension is necessary.
