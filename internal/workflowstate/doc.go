// Package workflowstate applies workflow manifests to a project's saved
// state: it reads saved runs as sources for reconciling a manifest, and
// imports a manifest into a project's queue under a project.Guard.
//
// `rotari import` and the MCP import tools share it, so both plan and write
// an import the same way. Callers supply what depends on their process: how
// a queue is validated against the known executors, and how a basedir is
// registered.
package workflowstate
