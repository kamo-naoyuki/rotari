package main

import (
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type attemptIDPayload = state.AttemptIDPayload

func makeAttemptID(runID, jobID string, number int) string {
	return state.MakeAttemptID(runID, jobID, number)
}

func decodeAttemptID(attemptID string) (attemptIDPayload, error) {
	return state.DecodeAttemptID(attemptID)
}

func latestAttemptID(runDir, jobID string) (string, error) {
	return state.LatestAttemptID(runDir, jobID)
}

func validatedStateFile(basePath, fileName string) (string, error) {
	return state.ValidatedStateFile(basePath, fileName)
}

func validatedJobDir(runDir, jobID string) (string, error) {
	return state.SafeJoin(runDir, jobID)
}

func validatedRunDir(paths pathSet, runID string) (string, error) {
	return state.SafeJoin(paths.RunsDir, runID)
}
