// Package run owns run planning and orchestration.
//
// It uses model values and caller-provided state to decide what work should
// execute, what results can be carried forward, how dependencies unblock work,
// when retries happen, and how lanes advance a run. It does not touch project
// files; internal/projectrun applies these rules to a project's on-disk state,
// and process or scheduler details stay behind executor boundaries.
package run
