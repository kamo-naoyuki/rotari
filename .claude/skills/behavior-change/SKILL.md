---
name: behavior-change
description: "Use when changing rotari behavior, especially job state, scheduling, queues, selection, result or status resolution, persistence, filesystem or path rules, CLI, server, Web UI/API, Python client, contracts, or conformance."
---

# Changing rotari behavior

Use this workflow for a behavior change, even when the code change appears
local. `AGENTS.md` rules (invariants, README sync, interfaces) still apply;
they are not repeated here.

1. Locate the affected command and package in `docs/ARCHITECTURE.md` and the
   relevant rows and notes in `contracts/README.md` by searching for their
   names; read only those sections, then the implementation and tests. Find
   every caller and sibling variant the rule touches.
2. Update all applicable interfaces and documentation, including
   `docs/ARCHITECTURE.md` when package, process, or command flows change.
3. For behavior visible through the binary or Web API, add or update a
   conformance test, preferring one table test across commands and variants
   with distinguishing fixture data. Keep `covers(t, "...")` IDs, the status
   table, and the contract note's implementation/test links consistent;
   assign an ID to a newly tested rule. Refactors keep conformance unchanged
   unless the contract changes.
4. Run the smallest relevant test with `-v` (a `-run` that matches nothing
   still passes), then the affected packages, then `go test ./conformance`
   for contract-visible changes. Show only failures and the summary
   (for example `2>&1 | tail -n 40`).
5. Run pre-commit on changed files, then `scripts/check.sh` once before
   finishing; skip `--short` when the full run follows.

Report any validation that could not be run and why.
