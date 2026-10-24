package main

import (
	"github.com/rotari-dev/rotari/internal/project"
	"github.com/rotari-dev/rotari/internal/state"
)

// ensureProjectIdle resolves a project and rejects operation unless it is
// idle; see project.EnsureIdle.
func ensureProjectIdle(baseDir, queueName, operation string) error {
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return err
	}
	return project.EnsureIdle(paths, operation)
}
