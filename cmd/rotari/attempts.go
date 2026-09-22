package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func makeAttemptID(runID, jobID string, number int) string {
	if !isValidPathElement(runID) || !isValidPathElement(jobID) || number < 0 {
		panic(fmt.Sprintf("invalid attempt ID components: run=%q job=%q number=%d", runID, jobID, number))
	}
	return fmt.Sprintf("att_%s-%s-%d", runID, jobID, number)
}

func decodeAttemptID(attemptID string) (attemptIDPayload, error) {
	if !strings.HasPrefix(attemptID, "att_") {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	value := strings.TrimPrefix(attemptID, "att_")
	separator := strings.LastIndexByte(value, '-')
	if separator <= 0 || separator == len(value)-1 {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	number, err := strconv.Atoi(value[separator+1:])
	if err != nil || number < 0 {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	core := value[:separator]
	if len(core) <= runIDLen || core[runIDLen] != '-' {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	payload := attemptIDPayload{RunID: core[:runIDLen], JobID: core[runIDLen+1:], Number: number}
	if !isValidPathElement(payload.RunID) || !isValidPathElement(payload.JobID) {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	return payload, nil
}

func attemptJobDir(runDir string, job JobSpec) (string, error) {
	jobDir, err := validatedJobDir(runDir, job.ID)
	if err != nil || job.AttemptID == "" {
		return jobDir, err
	}
	if !isValidPathElement(job.AttemptID) {
		return "", fmt.Errorf("invalid attempt ID %q", job.AttemptID)
	}
	return filepath.Join(jobDir, "attempts", job.AttemptID), nil
}

func latestAttemptJobDir(runDir, jobID string) (string, error) {
	jobDir, err := validatedJobDir(runDir, jobID)
	if err != nil {
		return "", err
	}
	attemptID, err := latestAttemptID(runDir, jobID)
	if err != nil || attemptID == "" {
		return jobDir, nil
	}
	return filepath.Join(jobDir, "attempts", attemptID), nil
}

func latestAttemptID(runDir, jobID string) (string, error) {
	jobDir, err := validatedJobDir(runDir, jobID)
	if err != nil {
		return "", err
	}
	runID := filepath.Base(runDir)
	entries, err := os.ReadDir(filepath.Join(jobDir, "attempts"))
	if err != nil {
		return "", err
	}
	latest := ""
	latestNumber := -1
	for _, entry := range entries {
		if !entry.IsDir() || !isValidPathElement(entry.Name()) {
			continue
		}
		payload, decodeErr := decodeAttemptIDForRun(entry.Name(), runID)
		if decodeErr != nil {
			payload, decodeErr = decodeAttemptID(entry.Name())
		}
		if decodeErr != nil || payload.JobID != jobID {
			continue
		}
		if payload.Number > latestNumber || (payload.Number == latestNumber && entry.Name() > latest) {
			latest = entry.Name()
			latestNumber = payload.Number
		}
	}
	return latest, nil
}

func decodeAttemptIDForRun(attemptID, runID string) (attemptIDPayload, error) {
	prefix := "att_" + runID + "-"
	if !strings.HasPrefix(attemptID, prefix) {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	value := strings.TrimPrefix(attemptID, prefix)
	separator := strings.LastIndexByte(value, '-')
	if separator <= 0 || separator == len(value)-1 {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	number, err := strconv.Atoi(value[separator+1:])
	if err != nil || number < 0 {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	jobID := value[:separator]
	if !isValidPathElement(runID) || !isValidPathElement(jobID) {
		return attemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	return attemptIDPayload{RunID: runID, JobID: jobID, Number: number}, nil
}

func specificAttemptJobDir(runDir, jobID, attemptID string) (string, error) {
	jobDir, err := validatedJobDir(runDir, jobID)
	if err != nil {
		return "", err
	}
	if !isValidPathElement(attemptID) {
		return "", fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	return filepath.Join(jobDir, "attempts", attemptID), nil
}
