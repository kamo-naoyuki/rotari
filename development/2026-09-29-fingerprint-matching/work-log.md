# Work History: Fingerprint Matching

See [plan.md](plan.md) for current scope and status. The original one-line commit notes did not record test commands/results; no test passes are inferred from tests appearing in diffs.

## Implement fingerprint-based matching

**Commits:** 2026-09-29 04:15:23 `3ddc57b`; 2026-09-29 15:01:24 `6db08a1`.

**Change:** Added fingerprint candidate matching to run planning and `--match-by`, and documented matching in filtered reruns.

**Reason:** Preserve continuity when job names or IDs change without treating IDs as content identities or overriding explicit provenance.

**Plan impact:** Landed the initial matching path and rerun contract.

**Validation:** Historical notes do not record test commands or results.

**Remaining:** The plan identifies match explanations and broader array/matrix conformance as follow-up areas; verify existing coverage before extending them.

## Make matching deterministic and test edge cases

**Commits:** 2026-10-01 21:45:55 `e58c9b2`; 2026-10-02 02:49:01 `c33ded9`; 2026-10-02 03:17:59 `8d4f748`; 2026-10-02 03:29:29 `6968919`.

**Change:** Stabilized match-group order and added tests for Job ID priority, fingerprint fallback, duplicate occurrence matching, metadata exclusion, missing history, normalized directories, and array ranges.

**Reason:** Matching must be deterministic and must not guess when duplicate counts or historical inputs are ambiguous.

**Plan impact:** Improved ordering and edge-case coverage.

**Validation:** The commits add focused tests; historical notes do not record execution results.

**Remaining:** Keep plan/dry-run explanations and any uncovered array/matrix integration cases as follow-up. No related issue was recorded.
