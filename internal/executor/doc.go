// Package executor defines job execution contracts and scheduler adapters.
//
// It owns the boundary for local process execution, scheduler submission,
// scheduler status resolution, cancellation, suspension, and wrapper scripts.
// It should execute or control jobs according to caller-provided specs without
// deciding queue selection, dependency scheduling, retry policy, or run
// finalization.
package executor
