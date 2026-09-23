package main

import (
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
