// Package run owns run planning and orchestration.
//
// It uses model values and caller-provided state to decide what work should
// execute, what results can be carried forward, how dependencies unblock work,
// when retries happen, and how workers and lanes advance a run. It should keep
// filesystem access behind state package boundaries and process or scheduler
// details behind executor boundaries.
package run
