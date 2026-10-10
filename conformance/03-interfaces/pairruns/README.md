# Run, recovery, wait, and note flag pairs

These tests belong to [interface conformance](../README.md). Run/retry pairs
only use dry-run previews; unlock and wait witnesses seed synthetic locks
once fixture execution has stopped, and note pairs add notes to a finished
fixture run. The shared fixture and bounded invocation
are in [conformance/support/pairs.go](../../support/pairs.go).

The suite is a separate Go package to stay below the default ten-minute
package timeout without excluding cases in short or race mode. Run all
interface suites using `go test ./conformance/03-interfaces/...`.
See [coverage](../../../docs/internal/testing/cli-flag-pair-coverage.md) for
safety boundaries and remaining gaps.
