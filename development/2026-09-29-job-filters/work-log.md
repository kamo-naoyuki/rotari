# Work History: Generalized Job Filters

See [plan.md](plan.md) for current scope and status. Historical one-line notes did not preserve test commands/results unless explicitly stated below; tests present in a diff are not treated as proof of a pass.

## Define filter syntax and shared CLI framework

**Commits:** 2026-09-29 20:53:53 `84d9f9e`; 2026-09-29 21:42:20 `cb585f7`; 2026-09-29 23:30:37 `557b87e`.

**Change:** Wrote the filter plan, standardized `--filter-*` names and negation syntax, and added CLI parsing/framework support with initial predicates.

**Reason:** Establish consistent syntax while keeping direct job selectors separate from filters.

**Plan impact:** Landed the initial design and framework.

**Validation:** Historical notes do not record test commands or results.

**Remaining:** Extend predicates and verify shared semantics at package and interface boundaries.

## Add execution, diagnosis, and definition predicates

**Commits:** 2026-09-30 01:00:58 `6e696d5`; 2026-09-30 01:09:35 `62983ec`; 2026-09-30 01:42:28 `20330b3`.

**Change:** Added execution-attribute and diagnosis selectors, then changed/new definition selectors, with CLI/Python metadata and documentation updates.

**Reason:** Expand selection beyond result status using explicit predicate families.

**Plan impact:** Advanced execution, diagnosis, and definition filters.

**Validation:** Changes include focused tests; historical notes do not record executed commands or results.

**Remaining:** Recheck generated client/reference output and carried-result, array, and matrix variants.

## Apply shared selection to control and run paths

**Commits:** 2026-09-30 02:11:19 `f979620`; 2026-10-02 00:26:22 `262dd2d`.

**Change:** Added name/filter target selection for `cancel`, `suspend`, and `resume`, then centralized individual selection in `jobfilter.Filter.Selects` and routed callers through it.

**Reason:** Prevent callers from implementing the same selection rule differently and ensure control requests act on an explicit target set.

**Plan impact:** Advanced filtered job control and established the shared selection owner.

**Validation:** Package/command tests were added; historical notes do not state which commands ran or passed.

**Remaining:** Keep contracts and callers aligned; assess whole-array selection per task rather than from aggregates.

## Verify cross-command selection and whole-array behavior

**Commits:** 2026-10-02 00:29:39 `2311d20`; 2026-10-02 00:49:07 `5751036`.

**Change:** Added conformance cases for `--filter-exit-code` across `show`, `copy`, `run`, and `retry`; implemented whole-array selection by aggregating task-level filter results and updated related tests/contracts.

**Reason:** The same rule must apply across commands and use each task's facts before combining an array outcome.

**Plan impact:** Strengthened cross-command conformance and array selection.

**Validation:** Conformance and package tests were added/updated; historical notes do not preserve execution results.

**Remaining:** Continue against the plan's remaining CLI/Python and sibling-interface coverage. Commit `2311d20` is also recorded in [the bug-prevention plan log](../2026-10-01-bug-prevention/work-log.md) because it advances both plans.
