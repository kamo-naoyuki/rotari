---
name: bug-fix
description: "Use when fixing a rotari bug or regression, including writing a reproducer, checking sibling command paths, or adding regression and conformance tests."
---

# Fixing a rotari bug

1. Trace the affected behavior through the implementation and existing tests.
   Use `docs/ARCHITECTURE.md` to find command paths; check the shared
   invariants in `AGENTS.md`.
2. Add a regression test that reproduces the report and run it before the
   fix. Confirm that it fails for the reported reason.
3. Fix the shared cause, not just the first failing caller. Check all
   applicable sibling variants and interfaces.
4. Run the regression test and relevant package tests. If the test was added
   after the fix, verify that it catches the bug on the pre-fix revision in a
   temporary `jj workspace` with an explicit base revision. Then forget that
   workspace and remove only its temporary directory.
5. If the bug is visible through the binary or Web API, add or update a
   conformance row and keep its contract ID and status table current.
6. Run `go test ./conformance` for contract-visible changes and
   `scripts/check.sh` before finishing. Report tests that could not be run.
