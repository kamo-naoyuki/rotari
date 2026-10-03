# Restored edit and mutation flag pairs

These tests belong to [interface conformance](../README.md). They exercise
queue/history edits from a restored fixture, with field and selector witnesses.
The shared fixture and NFS-aware restore are in
[conformance/support/pairs.go](../../support/pairs.go).

The suite is a separate Go package to stay below the default ten-minute
package timeout without excluding cases in short or race mode. Run all
interface suites using `go test ./conformance/03-interfaces/...`.
See [coverage](../flag-pair-coverage.md) for remaining gaps.
