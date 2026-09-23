# Rotari internals

This file is the entry point for Rotari's internal design notes. The detailed
implementation lives in the split pages under [docs/internals](internals/), and
cross-cutting rules remain here as the high-level index.

This is an architectural map, not a command reference. User-facing behavior
belongs in [../README.md](../README.md); implementation and tests remain in
code.

The current structural refactoring plan is documented in
[docs/REFACTORING_PLAN.md](REFACTORING_PLAN.md).

## Split notes

- [internals/00-overview.md](internals/00-overview.md): overview, system model,
  and core contracts.
- [internals/01-resolution-and-config.md](internals/01-resolution-and-config.md):
  path resolution, config precedence, shell completion, and registry behavior.
- [internals/02-run-lifecycle-and-execution.md](internals/02-run-lifecycle-and-execution.md):
  run creation, retries, filtered reruns, executor orchestration, and
  external integrations.
- [internals/03-server-and-command-interfaces.md](internals/03-server-and-command-interfaces.md):
  server projections, command interfaces, and client lifecycle.
- [internals/04-coordination-and-safety.md](internals/04-coordination-and-safety.md):
  locks, durability, recovery, and shared-state safety rules.
- [internals/05-web-assets-and-static-export.md](internals/05-web-assets-and-static-export.md):
  Web UI assets, static export, and embedded app boundaries.

## Cross-cutting rules to keep in sync

- Keep the filesystem as the source of truth; registry and in-memory state are
  only indexes or coordination helpers.
- Preserve the same fallback and resolution contracts across CLI, server, and
  web paths.
- Treat path elements as arbitrary strings and reject unsafe separators before
  filesystem access.
- Update user-facing docs and relevant tests whenever a behavior contract
  changes.
