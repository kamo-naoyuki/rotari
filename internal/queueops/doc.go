// Package queueops performs the queue and run-history edits that the CLI,
// the Web UI, and the supervisor share: add, change, remove, copy, and
// deleting runs.
//
// Each operation resolves the project's paths, follows the idle-edit
// sequence of internal/project, and returns the one-line message every
// interface reports. Pure queue transformations belong to internal/queueedit
// and the selector rules to internal/model; this package loads and saves the
// files around them. It must not parse flags, prompt, or print.
package queueops
