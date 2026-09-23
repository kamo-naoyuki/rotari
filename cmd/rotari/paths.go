package main

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type attemptIDPayload = state.AttemptIDPayload

func makeAttemptID(runID, jobID string, number int) string {
	return state.MakeAttemptID(runID, jobID, number)
}

func decodeAttemptID(attemptID string) (attemptIDPayload, error) {
	return state.DecodeAttemptID(attemptID)
}

func attemptJobDir(runDir string, job JobSpec) (string, error) {
	return state.AttemptJobDir(runDir, model.JobSpec(job))
}

func latestAttemptJobDir(runDir, jobID string) (string, error) {
	return state.LatestAttemptJobDir(runDir, jobID)
}

func latestAttemptID(runDir, jobID string) (string, error) {
	return state.LatestAttemptID(runDir, jobID)
}

func specificAttemptJobDir(runDir, jobID, attemptID string) (string, error) {
	return state.SpecificAttemptJobDir(runDir, jobID, attemptID)
}

func isValidPathElement(value string) bool {
	return state.IsValidPathElement(value)
}

func joinValidatedPath(basePath, element string) (string, error) {
	if !isValidPathElement(element) {
		return "", fmt.Errorf("invalid path element %q", element)
	}
	return state.SafeJoin(basePath, element)
}

func validatedStateFile(basePath, fileName string) (string, error) {
	return state.ValidatedStateFile(basePath, fileName)
}

func validatedJobDir(runDir, jobID string) (string, error) {
	return joinValidatedPath(runDir, jobID)
}

func validatedRunDir(paths pathSet, runID string) (string, error) {
	return joinValidatedPath(paths.RunsDir, runID)
}

func isValidProjectName(projectName string) bool {
	return isValidPathElement(projectName)
}
