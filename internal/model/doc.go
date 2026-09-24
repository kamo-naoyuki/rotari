// Package model defines Rotari's domain data structures and pure domain rules.
//
// It owns concepts such as queues, queued commands, job specs, run summaries,
// job results, selections, dependencies, and array jobs. It should not depend on
// filesystem paths, locks, executors, CLI adapters, server adapters, or Web
// projections.
package model
