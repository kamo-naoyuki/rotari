// Package queueedit applies edits to a project's queue as pure
// transformations.
//
// It owns the rules for building a queue from an earlier run's command
// snapshot: which jobs a selection copies, which omitted prerequisites must
// have succeeded, how stage and matrix dependencies are kept or rewritten,
// and what origin each copied job records. Callers load and persist state,
// hold the locks, and validate attempt directories; this package must not
// touch the filesystem.
package queueedit
