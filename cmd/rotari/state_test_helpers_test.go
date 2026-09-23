package main

import "github.com/kamo-naoyuki/rotari/internal/state"

func defaultMeta() Meta {
	return Meta(state.DefaultMeta())
}

func loadMeta(path string) (Meta, error) {
	meta, err := state.LoadMeta(path)
	return Meta(meta), err
}

func makeAttemptID(runID, jobID string, number int) string {
	return state.MakeAttemptID(runID, jobID, number)
}

func loadRunSummary(path string) (RunSummary, error) {
	summary, err := state.LoadRunSummary(path)
	return RunSummary(summary), err
}

func loadQueue(path string) (Queue, error) {
	queue, err := state.LoadQueue(path)
	return Queue(queue), err
}

func writeJSON(path string, value any) error {
	return state.WriteJSON(path, value)
}

func latestAttemptJobDir(runDir, jobID string) (string, error) {
	return state.LatestAttemptJobDir(runDir, jobID)
}

func specificAttemptJobDir(runDir, jobID, attemptID string) (string, error) {
	return state.SpecificAttemptJobDir(runDir, jobID, attemptID)
}

func decodeAttemptID(attemptID string) (state.AttemptIDPayload, error) {
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
