# Internal documentation

Developer-facing reference material and maintained test notes live here. These
pages are not part of the user guide.

## Architecture

- [Code architecture](../ARCHITECTURE.md) maps processes, packages, and command
  flows. It is kept at its current path because contracts and repository
  instructions link to it as the canonical architecture map.
- [Technology rationale](architecture/design-rationale.md) records internal
  implementation choices without making them user-facing compatibility
  promises.

## Contract implementation notes

- [Basedir discovery registry](contracts/basedir-discovery.md) records the
  internal registry design separated from the resolution and garbage
  collection behavior specified by the contracts.

## Testing and CI

- [Scheduler integration coverage](testing/scheduler-integration.md) states
  what scheduler validation exercises and what it does not certify.
- [CLI flag-pair coverage](testing/cli-flag-pair-coverage.md) documents the
  interface conformance matrix, witnesses, and known observation gaps.
- [CLI flag-pair implementation triage](testing/cli-flag-pair-triage.md)
  preserves the rollout findings separately from the maintained coverage
  guide.
- [Selector conformance fixture](testing/selector-fixture.md) documents the
  shared fixture layout used by selector tests.
- Test-group and package-specific instructions remain beside their suites
  under [conformance](../../conformance/); these local READMEs describe how to
  run and navigate the code next to them.
