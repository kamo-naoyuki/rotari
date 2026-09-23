package main

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func isValidPathElement(value string) bool {
	return state.IsValidPathElement(value)
}

func joinValidatedPath(basePath, element string) (string, error) {
	if !isValidPathElement(element) {
		return "", fmt.Errorf("invalid path element %q", element)
	}
	return state.SafeJoin(basePath, element)
}

func safeJoin(basePath, element string) (string, error) {
	return state.SafeJoin(basePath, element)
}

func validatedStateFile(basePath, fileName string) (string, error) {
	return state.ValidatedStateFile(basePath, fileName)
}

func validatedJobDir(runDir, jobID string) (string, error) {
	return joinValidatedPath(runDir, jobID)
}

func validatedRunDir(paths pathSet, runID string) (string, error) {
	return joinValidatedPath(paths.runsDir, runID)
}

func isValidProjectName(projectName string) bool {
	return isValidPathElement(projectName)
}
