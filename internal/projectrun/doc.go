// Package projectrun runs one project's queue against its on-disk state.
//
// It owns the run lifecycle that the synchronous supervisor run, the async
// worker, and cancellation finalization share: Begin records the run and marks
// the project running, Execute plans and executes the queued jobs through
// internal/run and writes the run summary, and Finish records final context,
// clears the consumed queue, finalizes metadata, and releases the run lock.
//
// Planning, dependency, retry, and lane rules stay in internal/run, file
// layout in internal/state, and job execution in internal/executor. Services
// that live outside the project directory, such as the run registry, config
// discovery, webhooks, and failure diagnosis, are supplied by the caller.
package projectrun
