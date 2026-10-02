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

If substantial work was already underway or completed before a plan was written, a `plan.md` is optional and may be omitted. Still create a directory for that work and record its relevant commits in `WORK_LOG.md`. In this case, use the date of the first related commit as the directory prefix, so the directory records when the work began. Do not create directories or log small, isolated fixes that do not warrant a plan.

Example layout:

```text
development/
  README.md
  ISSUES.md
  2026-10-02-example-feature/
    plan.md
    work-log.md
  2026-10-01-existing-substantial-work/
    work-log.md
```

## Recording related commits

Keep the current overview in each plan's `plan.md`. Record implementation
history in that plan directory's `work-log.md`, grouped by cohesive change
rather than creating one tiny file per commit. This keeps each plan's history
separate while gathering related decisions and outcomes together.

Each entry must include the actual Git date and time for every related commit,
as well as its short ID. Use the timestamp from Git, not the note-writing date.
Do not copy commit messages verbatim. Explain the work in context and cover:

- **Change:** affected packages, commands, interfaces, tests, and documents.
- **Reason:** the plan decision, milestone, or problem addressed.
- **Plan impact:** milestone advanced and decisions made, reversed, or opened.
- **Validation:** commands/checks actually run and their results. If historical
  evidence is unavailable, state that explicitly; do not infer a pass from a
  test existing in the diff.
- **Remaining:** known follow-up and related `ISSUES.md` entries, or "None".

Group commits only when they form one coherent change; give separate entries
when the reason, validation, or remaining work differs. Add the entry after the
commit exists so its ID and timestamp are known. Do not record unrelated
commits. A commit may appear in multiple plan logs only when it materially
advances each one, with plan-specific context. Keep old plan directories named
for the date the work began; do not rename them when updating the log.

For substantial work begun before a plan existed, use the date of its first
related commit as the directory prefix. A `plan.md` is optional for completed
work that was never planned, but its history belongs in that work directory's
`work-log.md`.
