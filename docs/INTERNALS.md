# Rotari internals

This file is the entry point for Rotari's internal design notes. The detailed
implementation lives in the split pages under [docs/internals](internals/), and
cross-cutting rules remain here as the high-level index.

This is an architectural map, not a command reference. User-facing behavior
belongs in [../README.md](../README.md) and the user guides it links to under
`docs/`; implementation and tests remain in
code.

When a behavior contract changes, update the relevant internal note in the
same change. Include links to the representative implementation and tests so
future contributors and coding agents can move from the contract to the code
quickly.

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
  only indexes or coordination helpers. The main persistence boundary is
  [`internal/state/store.go`](../internal/state/store.go), with coverage in
  [`internal/state/store_test.go`](../internal/state/store_test.go).
- Preserve the same fallback and resolution contracts across CLI, server, and
  web paths. Start with [`cmd/rotari/show.go`](../cmd/rotari/show.go),
  [`cmd/rotari/report.go`](../cmd/rotari/report.go), and
  [`cmd/rotari/web.go`](../cmd/rotari/web.go); representative tests are in
  [`cmd/rotari/show_test.go`](../cmd/rotari/show_test.go) and
  [`cmd/rotari/web_test.go`](../cmd/rotari/web_test.go).
- Treat path elements as arbitrary strings and reject unsafe separators before
  filesystem access. The shared boundary is
  [`internal/state/paths.go`](../internal/state/paths.go), with path safety
  checks in [`cmd/rotari/main_test.go`](../cmd/rotari/main_test.go).
- Update user-facing docs and relevant tests whenever a behavior contract
  changes. The user-facing entry points are [`README.md`](../README.md), the
  user guides it links to under `docs/`, and [`docs/FAQ.md`](FAQ.md); keep the relevant package tests alongside the
  implementation change.
