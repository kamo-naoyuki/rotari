package main

import (
	"flag"
	"strings"

	"github.com/rotari-dev/rotari/internal/resolve"
	"github.com/rotari-dev/rotari/internal/state"
)

// File defaults are not explicit registry constraints. Carry that provenance
// across CLI flag parsing, while preserving explicit CLI/environment selectors.
func configRegistrySelectors(base, project, runID string) (string, string, error) {
	if runID == "" {
		return base, project, nil
	}
	location, found, err := resolveRunLocation(runID)
	if err != nil || !found {
		return base, project, err
	}
	if !cliLocationExplicit["basedir"] && base != "" && base == cliLocationDefaults["basedir"] {
		base = location.BaseDir
	}
	if !cliLocationExplicit["project-name"] && project != "" && project == cliLocationDefaults["project-name"] {
		project = location.ProjectName
	}
	return base, project, nil
}

func resolveCLIExistingRun(base, project, runID string) (string, string, error) {
	base, project, err := configRegistrySelectors(base, project, runID)
	if err != nil {
		return "", "", err
	}
	return resolve.ExistingRun(base, project, runID)
}

func resolveCLIExistingRunID(base, project, runID string) (string, string, string, error) {
	base, project, err := configRegistrySelectors(base, project, runID)
	if err != nil {
		return "", "", "", err
	}
	return resolve.ExistingRunID(base, project, runID)
}

func resolveCLIAttempt(attempt, base, project, runID string) (string, string, string, string, error) {
	payload, err := state.DecodeAttemptID(attempt)
	if err != nil {
		return "", "", "", "", err
	}
	base, project, err = configRegistrySelectors(base, project, payload.RunID)
	if err != nil {
		return "", "", "", "", err
	}
	return resolve.Attempt(attempt, base, project, runID)
}

func resolveCLIJobSelection(scope resolve.Scope, project string, ids []string) (resolve.JobControl, error) {
	for _, id := range ids {
		runID := id
		if strings.HasPrefix(id, "att_") {
			payload, err := state.DecodeAttemptID(id)
			if err != nil {
				return resolve.JobControl{}, err
			}
			runID = payload.RunID
		}
		if resolve.IsRunID(runID) {
			var err error
			scope.BaseDir, project, err = configRegistrySelectors(scope.BaseDir, project, runID)
			if err != nil {
				return resolve.JobControl{}, err
			}
			break
		}
	}
	return resolve.JobSelection(scope, project, ids)
}

// cliScope is where a command looks for a selector: across the state
// directories for a read-only command without --basedir, and otherwise in
// the resolved one.
func cliScope(fs *flag.FlagSet, baseDir string) resolve.Scope {
	return resolve.Scope{
		BaseDir:         baseDir,
		Pinned:          !crossBaseDirCommands[fs.Name()] || cliOptionSet(fs, "basedir"),
		ExplicitProject: cliOptionSet(fs, "project-name"),
	}
}
