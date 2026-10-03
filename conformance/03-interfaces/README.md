# Interface conformance

Conformance tests for [contracts/03-server-and-command-interfaces.md](../../contracts/03-server-and-command-interfaces.md)
belong here.

The long flag-pair suites run in child packages [pairedits](pairedits/),
[pairruns](pairruns/), [pairweb](pairweb/), and
[pairjobcontrol](pairjobcontrol/),
sharing [support/pairs.go](../support/pairs.go).
The contract-to-directory mapping stays at this group in
[layout.json](../layout.json); contract tests scan all child packages.
Use `go test ./conformance/03-interfaces/...` to include every suite.

See [CLI flag-pair coverage](flag-pair-coverage.md) for generated pairs, current
execution/observation coverage, reasoned equivalences, and deferred adapters.
