# CLI flag-pair implementation triage

This is a historical record of notable findings from the staged rollout of
CLI flag-pair conformance. It is not a statement of current test coverage; see
[coverage and remaining gaps](cli-flag-pair-coverage.md) for the maintained
suite description. The complete work history lives in the
[CLI option interactions plan](../../../development/2026-10-03-cli-option-interactions/plan.md)
and its work log.

Initial harness failures were an assumed single ID for matrix `add`, an invalid
diagnosis sample, missing queue scopes, and incomplete classification of
existing explicit mode rejections. These were repaired, not allowlisted.

The first semantic run exposed a confirmed bug in `show --logs`: both `--failed`
and `--filter-result failed` were accepted but ignored. The existing log
selector now receives the parsed failed selection. Focused unit and external
regressions failed before the fix, including carried output. See the record in
[development/ISSUES.md](../../../development/ISSUES.md).

The JSON/failed witness also failed against `fddf05a`, immediately before the
historical JSON selection fix, with a job-ID mismatch (not a build failure).
Only current test code and test-harness support were copied into that temporary
worktree; the old production CLI was unchanged. The worktree was removed.

After the fix, a measured full pair run accepted 463 pairs and explicitly
rejected 299, with no unclassified/order failures. Its 1,524 invocations took
29.84 seconds including setup (other runs under concurrent load were slower).
This is an observation, not a fixed CI runtime bound.

The next expansion added `check`, `lineage`, `config`, and `export`: 57 more
pairs (50 accepted, 7 explicitly rejected). The combined measured runs accepted
513 and rejected 306 of 819 pairs with 1,638 invocations. Read-only pairs took
38.25 seconds including setup, and file pairs took 6.40 seconds including their
own fixture. No new production bug or equality exception was found. No command
compatibility declaration was added; implementation behavior is unchanged.

The mutation expansion added `remove`, `reset`, and `delete`: 178 pairs (158
accepted, 20 explicitly rejected), 41 seconds with the differential restore.
No option was found silently ignored and no production code changed. Triage
found one diagnostic gap, recorded as an open issue: `remove` rejects two
selector kinds, including `--all` with `--filter-stage`, with only its usage
line. Two harness defects were repaired, not allowlisted: the fixture assumed a
supervisor was still running for `server shutdown`, and the first exclusion
sample excluded a stage no selector picks, so it could not detect an ignored
`--filter-not-stage`.

The edit expansion added `add`, `change`, `copy`, and `import`: 1,520 pairs
(1,187 accepted, 333 explicitly rejected) without a production bug or equality
exception. Initial failures were harness defects and were corrected: standalone
samples now restore state, preventing prior edits from clearing later matrix
selectors; add uses a name that does not collide with a matrix base name;
dependency validation failures are classified narrowly; import revisions are
verified before normalization; and newly allocated IDs are normalized without
rewriting existing IDs. `TestCLIFlagPairEditSamples` exercises every individual
schema flag. Tests for definition fields, copy selector intersection, JSON
preview, and `--quiet` provide semantic witnesses; the remaining cases are not
claimed as ignored-option coverage.

The run/retry preview expansion executes all 3,315 pairs: 3,016 accepted and
299 explicitly rejected in both orders (6,630 invocations, 88.95 seconds with
four concurrent pair cases). It found `--async` silently accepted with
`--dry-run` for both commands in both orders. Before the fix, all four
regression cases returned exit 0 and printed a preview. `runJobs` now rejects
the combination before planning; `TestCLIFlagPairAsyncDryRunIsRejected` checks
the exact error and unchanged state. The result/stage and partial-array plan
witnesses pass. These remain previews; no job or scheduler is started.

A run-name witness also failed before the fix: the requested name was missing
from the preview. The shared CLI now includes `run_name=NAME` in the preview
summary without persisting or reserving it (CLI-10). The selector/partial-array
witness currently verifies `run`; sibling `retry` semantic coverage remains a
follow-up, although both commands participate in the generated pair matrix.
