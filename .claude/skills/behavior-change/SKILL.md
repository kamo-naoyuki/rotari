---
name: behavior-change
description: "Use when changing rotari behavior, especially job state, scheduling, queues, selection, result or status resolution, persistence, filesystem or path rules, CLI, server, Web UI/API, Python client, contracts, or conformance."
---

# Changing rotari behavior

Use this workflow for a behavior change, even when the code change appears
local.

1. Read `docs/ARCHITECTURE.md`, then inspect the implementation and tests for
   the affected command and package.
2. Read `contracts/README.md` and the relevant contract notes for changes to
   job state, scheduling, queues, results/status, persistence, filesystem,
   CLI, or Web/API behavior.
3. Check the project invariants in `AGENTS.md`. Find every caller and sibling
   variant affected by the rule. Keep shared rules in their owning package;
   do not duplicate them in an interface adapter.
4. Update all applicable interfaces and documentation, including
   `docs/ARCHITECTURE.md` when package, process, or command flows change. Do
   not edit `README.md` directly; update its source document and run
   `python3 scripts/sync_readme.py` when its generated section changes.
5. For behavior visible through the binary or Web API, add or update a
   conformance test. Keep `covers(t, "...")` IDs and the contract status
   table in `contracts/README.md` consistent; assign an ID if a newly tested
   contract rule does not have one. Keep the relevant contract note linked to
   its representative implementation and tests. Refactors must keep
   conformance passing without changing it unless the contract itself changes.
6. For shared behavior, prefer table tests that exercise each command and
   variant with distinguishing fixture data, such as different exit codes,
   multiple failing tasks, host/no-host cases, and carried/executed results.
7. Run the smallest relevant test first, then affected package tests.
   `go test -run` can report success when no tests matched; use `-v` or
   confirm the selected tests. For contract-visible changes, run
   `go test ./conformance`.
8. Run pre-commit on changed files for formatting, then
   `scripts/check.sh --short`. Before finishing, run `scripts/check.sh`.

Report any validation that could not be run and why.
