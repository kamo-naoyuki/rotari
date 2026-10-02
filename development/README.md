# Development Notes

This directory contains internal work tracking for issues, ongoing plans, and the commits that advance each plan. It complements, but does not replace, contracts, user documentation, or GitHub issues.

## Tracking issues

Use [ISSUES.md](ISSUES.md) for bugs, design concerns, and technical debt discovered during development.

- Add unresolved items under `Open`. Keep each entry concise and actionable; include the affected area, the condition or steps that reproduce it, and expected versus actual behavior when known.
- Record the root cause, relevant invariant, regression test, and affected interfaces when those details are available. Do not guess at a cause that has not been established.
- Do not use this list as a backlog for planned features or as a replacement for GitHub issues.
- When an item is fixed, remove it or move a short useful record to `Resolved`. Keep the resolved section concise.

## Creating a plan directory

Create one directory per cohesive work item under `development/`, prefixed with the date the plan is written and followed by a lowercase kebab-case name: `development/YYYY-MM-DD-example-feature/`. Add `plan.md` as the entry point and put the creation date near its title. Write plan content in English and include, as appropriate:

- purpose, scope, and non-goals;
- decisions and constraints;
- implementation approach or phases;
- current status and remaining work;
- tests and validation needed for completion.

Prefer a focused plan over adding a section to an unrelated plan. Link to related plans with relative Markdown links. Update the plan when implementation changes its assumptions or materially advances its status.

Example layout:

```text
development/
  README.md
  ISSUES.md
  2026-10-02-example-feature/
    plan.md
    2026-10-02_173455_a1b2c3d.md
```

## Recording related commits

For every commit that materially advances a plan, add a short English Markdown note in that plan's directory. Name the file using the commit timestamp and short commit ID:

```text
YYYY-MM-DD_HHMMSS_<short-commit-id>.md
```

For example, `2026-10-02_173455_a1b2c3d.md`. Use the timestamp recorded for that commit in Git; do not estimate it or use the date the note is written. This keeps filenames sortable in chronological order. Summarize the change in one or a few sentences, and mention the plan-relevant outcome rather than copying the full commit message.

Create the note after the commit exists so its ID and timestamp are known. Do not create one for unrelated commits. A commit may be noted in more than one plan only when it materially advances each plan; write a summary appropriate to each plan. Keep `plan.md` as the current overview, and use timestamped notes for the chronological record.

For plans that predate this naming convention, use the date of the commit that first added the plan as the directory prefix and record that date in `plan.md`. Do not rename an existing plan directory to the date of a later edit.
