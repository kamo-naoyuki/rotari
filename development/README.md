# Development Notes

This directory contains internal work tracking for issues, ongoing plans, and the commits that advance each plan. It complements, but does not replace, contracts, user documentation, or GitHub issues.

## Tracking issues

Use [ISSUES.md](ISSUES.md) for bugs, design concerns, and technical debt discovered during development.

- Add unresolved items under `Open`. Keep each entry concise and actionable; include the affected area, the condition or steps that reproduce it, and expected versus actual behavior when known.
- Record the root cause, relevant invariant, regression test, and affected interfaces when those details are available. Do not guess at a cause that has not been established.
- Do not use this list as a backlog for planned features or as a replacement for GitHub issues.
- When an item is fixed, remove it or move a short useful record to `Resolved`. Keep the resolved section concise.

## Creating a plan directory

Create a dated directory for substantial, cohesive work (such as a multi-commit feature or refactoring), not for every small fix. Use a lowercase kebab-case name after the date: `development/YYYY-MM-DD-example-feature/`. When planning before implementation, use the date the plan is written, add `plan.md` as the entry point, and put the creation date near its title. Write plan content in English and include, as appropriate:

- purpose, scope, and non-goals;
- decisions and constraints;
- implementation approach or phases;
- current status and remaining work;
- tests and validation needed for completion.

Prefer a focused plan over adding a section to an unrelated plan. Link to related plans with relative Markdown links. Update the plan when implementation changes its assumptions or materially advances its status.

### Work started before a plan exists

If substantial work was already underway or completed before a plan was written, a `plan.md` is optional and may be omitted. Still create a directory for that work and record its relevant commits using the timestamped commit-note format below. In this case, use the date of the first related commit as the directory prefix, so the directory records when the work began. Do not create directories or commit notes for small, isolated fixes that do not warrant a plan.

Example layout:

```text
development/
  README.md
  ISSUES.md
  2026-10-02-example-feature/
    plan.md
    2026-10-02_173455_a1b2c3d.md
  2026-10-01-existing-substantial-work/
    2026-10-01_091500_d4e5f6a.md
```

## Recording related commits

For every commit that materially advances a plan, add a short English Markdown note in that plan's directory. Name the file using the commit timestamp and short commit ID:

```text
YYYY-MM-DD_HHMMSS_<short-commit-id>.md
```

For example, `2026-10-02_173455_a1b2c3d.md`. Use the timestamp recorded for that commit in Git; do not estimate it or use the date the note is written. This keeps filenames sortable in chronological order.

A note must let a reader understand the commit's place in the plan without opening the diff. Do not copy the full commit message, and do not reduce the note to a single summary sentence. Every note must cover:

- **Change:** what changed, naming the affected packages, commands, interfaces, and documents.
- **Reason:** why the change was made, including the plan section, decision, or problem it addresses.
- **Plan impact:** which phase or milestone advanced, and any decision made, reversed, or newly opened.
- **Validation:** the tests or checks that were run and their results, or why none were run.
- **Remaining:** follow-up work, known gaps, and issues recorded in `ISSUES.md` because of this change; write "None" when there are none.

For example:

```markdown
# Move job inspection into a protocol-neutral package

## Change
Moved `GetJobInfo` and its request/response types from `internal/mcp` to `internal/agentapi`. `cmd/mcp/agent` now depends only on `internal/agentapi`; `internal/mcp` maps the MCP tool to it. Updated `docs/ARCHITECTURE.md`.

## Reason
M0 of the plan: `rotari-agent` imported the MCP SDK through `internal/mcp`, contradicting the protocol-neutral core principle.

## Plan impact
M0 item 1 is done. No decisions changed.

## Validation
`go test ./internal/agentapi ./internal/mcp ./cmd/mcp/...` and `scripts/check.sh` passed.

## Remaining
Basedir references and structured output (M0 items 2-3).
```

Create the note after the commit exists so its ID and timestamp are known. Do not create one for unrelated commits. A commit may be noted in more than one plan only when it materially advances each plan; write a summary appropriate to each plan. Keep `plan.md` as the current overview, and use timestamped notes for the chronological record.

For plans that predate this naming convention, use the date of the commit that first added the plan as the directory prefix and record that date in `plan.md`. Do not rename an existing plan directory to the date of a later edit. For substantial work that began before a plan existed, use the date of its first related commit as described above; no `plan.md` is required.
