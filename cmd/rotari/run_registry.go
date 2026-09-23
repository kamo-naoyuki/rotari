package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

type runLocation struct {
	BaseDir     string `json:"base_dir"`
	ProjectName string `json:"project_name"`
	RunID       string `json:"run_id"`
}

func resolveAttemptTarget(attemptID, cliBaseDir, cliProjectName, cliRunID string) (string, string, string, string, error) {
	payload, err := state.DecodeAttemptID(attemptID)
	if err != nil {
		return "", "", "", "", err
	}
	if cliRunID != "" && cliRunID != payload.RunID {
		return "", "", "", "", fmt.Errorf("attempt %q belongs to run %q, not %q", attemptID, payload.RunID, cliRunID)
	}
	baseDir, projectName, err := resolveExistingRunTarget(cliBaseDir, cliProjectName, payload.RunID)
	if err != nil {
		return "", "", "", "", err
	}
	return baseDir, projectName, payload.RunID, payload.JobID, nil
}

func registerRun(paths pathSet, runID string) error {
	return registerRunLocation(runLocation{
		BaseDir: paths.BaseDir, ProjectName: paths.ProjectName, RunID: runID,
	})
}

func registerRunLocation(location runLocation) error {
	baseDir, err := filepath.Abs(location.BaseDir)
	if err != nil {
		return err
	}
	location.BaseDir = baseDir
	dir, err := runRegistryDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, stateDirMode()); err != nil {
		return err
	}
	path, err := runLocationPath(dir, location.RunID)
	if err != nil {
		return err
	}
	var existing runLocation
	if err := jsonStore().ReadJSON(path, &existing); err == nil {
		if existing != location {
			return fmt.Errorf("run id %q is already registered to another location", location.RunID)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("run id %q is already registered to another location", location.RunID)
	}
	return state.WriteJSON(path, location)
}

func unregisterRun(runID string) error {
	dir, err := runRegistryDir()
	if err != nil {
		return err
	}
	path, err := runLocationPath(dir, runID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func resolveRunLocation(runID string) (runLocation, bool, error) {
	dir, err := runRegistryDir()
	if err != nil {
		return runLocation{}, false, err
	}
	path, err := runLocationPath(dir, runID)
	if err != nil {
		return runLocation{}, false, err
	}
	var location runLocation
	if err := jsonStore().ReadJSON(path, &location); err != nil {
		if os.IsNotExist(err) {
			return runLocation{}, false, nil
		}
		return runLocation{}, false, fmt.Errorf("invalid run registry entry for %q: %w", runID, err)
	}
	if location.BaseDir == "" || location.ProjectName == "" || location.RunID != runID {
		return runLocation{}, false, fmt.Errorf("invalid run registry entry for %q", runID)
	}
	return location, true, nil
}

func resolveExistingRunTarget(cliBaseDir, cliProjectName, runID string) (string, string, error) {
	if runID != "" {
		location, found, err := resolveRunLocation(runID)
		if err != nil {
			return "", "", err
		}
		if found {
			if cliBaseDir != "" {
				baseDir, err := filepath.Abs(cliBaseDir)
				if err != nil {
					return "", "", err
				}
				if baseDir != location.BaseDir {
					return "", "", fmt.Errorf("run %q is registered under basedir %q, not %q", runID, location.BaseDir, baseDir)
				}
			} else {
				cliBaseDir = location.BaseDir
			}
			if cliProjectName != "" && cliProjectName != location.ProjectName {
				return "", "", fmt.Errorf("run %q is registered under project %q, not %q", runID, location.ProjectName, cliProjectName)
			}
			if cliProjectName == "" {
				cliProjectName = location.ProjectName
			}
			if !runLocationExists(location) {
				return "", "", fmt.Errorf("run %q is registered but its run directory is missing; run 'rotari gc' to inspect stale registry entries", runID)
			}
		}
	}
	baseDir, _, err := resolveBaseDir(cliBaseDir)
	if err != nil {
		return "", "", err
	}
	projectName, err := resolveProjectName(baseDir, cliProjectName)
	if err != nil {
		return "", "", err
	}
	return baseDir, projectName, nil
}

func runRegistryDir() (string, error) {
	masterDir, err := resolveMasterDir("")
	if err != nil {
		return "", err
	}
	return filepath.Join(masterDir, "runs"), nil
}

func runLocationPath(dir, runID string) (string, error) {
	if !state.IsValidPathElement(runID) {
		return "", fmt.Errorf("invalid run id %q", runID)
	}
	return filepath.Join(dir, runID+".json"), nil
}
