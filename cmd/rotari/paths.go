package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func isValidPathElement(value string) bool {
	if value == "" || value == "." || value == ".." || filepath.IsAbs(value) {
		return false
	}
	if strings.ContainsAny(value, `/\\`) {
		return false
	}
	return filepath.Base(value) == value
}

func joinValidatedPath(basePath, element string) (string, error) {
	if !isValidPathElement(element) {
		return "", fmt.Errorf("invalid path element %q", element)
	}
	return safeJoin(basePath, element)
}

func safeJoin(basePath, element string) (string, error) {
	root := filepath.Clean(basePath)
	// NOSONAR: element is validated as a single path element before it reaches this join.
	joined := filepath.Join(root, element)
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path element %q", element)
	}
	return joined, nil
}

func validatedStateFile(basePath, fileName string) (string, error) {
	switch fileName {
	case stateFileCommandsJSON, stateFileSummaryJSON, stateFileContextJSON, stateFileOutput,
		stateFileSchedulerJSON, stateFileStatusJSON, stateFileStatus, stateFileSubmittedAt,
		stateFileFinishedAt, commandJSONName, stateFileJobJSON, stateFilePID, stateFileCancelled,
		stateFileName:
		return safeJoin(basePath, fileName)
	default:
		return "", fmt.Errorf("invalid state file name %q", fileName)
	}
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
