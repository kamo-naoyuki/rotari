package main

import (
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// registerRun records where a new run lives in the default run registry.
func registerRun(paths state.ProjectPaths, runID string) error {
	registry, err := runregistry.Default()
	if err != nil {
		return err
	}
	return registry.Register(runregistry.Location{BaseDir: paths.BaseDir, ProjectName: paths.ProjectName, RunID: runID})
}

// unregisterRun removes a deleted run from the default run registry.
func unregisterRun(runID string) error {
	registry, err := runregistry.Default()
	if err != nil {
		return err
	}
	return registry.Unregister(runID)
}

// resolveRunLocation looks runID up in the default run registry.
func resolveRunLocation(runID string) (runregistry.Location, bool, error) {
	registry, err := runregistry.Default()
	if err != nil {
		return runregistry.Location{}, false, err
	}
	return registry.Lookup(runID)
}
