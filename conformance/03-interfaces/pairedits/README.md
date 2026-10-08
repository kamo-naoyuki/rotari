# Restored edit and mutation flag pairs

These tests belong to [interface conformance](../README.md). They exercise
queue/history edits from a restored fixture, with field and selector witnesses.
The shared fixture and NFS-aware restore are in
[conformance/support/pairs.go](../../support/pairs.go).

The suite is a separate Go package, with no cases excluded in short or race
mode. Its 1,648 flag pairs each run in both orders; under full-repository race
load, the package can exceed Go's default ten-minute timeout. The repository
check and CI use a 15-minute race-test timeout for this reason. To run the
package's race tests directly, use `go test -race -timeout 15m
./conformance/03-interfaces/pairedits`. Run all interface suites using `go test
./conformance/03-interfaces/...`.
See [coverage](../flag-pair-coverage.md) for remaining gaps.
