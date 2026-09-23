package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

const runIDLength = len("20060102-150405-00000000")

type AttemptIDPayload struct {
	RunID  string
	JobID  string
	Number int
}

func MakeAttemptID(runID, jobID string, number int) string {
	if !IsValidPathElement(runID) || !IsValidPathElement(jobID) || number < 0 {
		panic(fmt.Sprintf("invalid attempt ID components: run=%q job=%q number=%d", runID, jobID, number))
	}
	return fmt.Sprintf("att_%s-%s-%d", runID, jobID, number)
}

func DecodeAttemptID(attemptID string) (AttemptIDPayload, error) {
	if !strings.HasPrefix(attemptID, "att_") {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	value := strings.TrimPrefix(attemptID, "att_")
	separator := strings.LastIndexByte(value, '-')
	if separator <= 0 || separator == len(value)-1 {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	number, err := strconv.Atoi(value[separator+1:])
	if err != nil || number < 0 {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	core := value[:separator]
	if len(core) <= runIDLength || core[runIDLength] != '-' {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	payload := AttemptIDPayload{RunID: core[:runIDLength], JobID: core[runIDLength+1:], Number: number}
	if !IsValidPathElement(payload.RunID) || !IsValidPathElement(payload.JobID) {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	return payload, nil
}

func AttemptJobDir(runDir string, job model.JobSpec) (string, error) {
	jobDir, err := SafeJoin(runDir, job.ID)
	if err != nil || job.AttemptID == "" {
		return jobDir, err
	}
	if !IsValidPathElement(job.AttemptID) {
		return "", fmt.Errorf("invalid attempt ID %q", job.AttemptID)
	}
	return filepath.Join(jobDir, "attempts", job.AttemptID), nil
}

func LatestAttemptJobDir(runDir, jobID string) (string, error) {
	jobDir, err := SafeJoin(runDir, jobID)
	if err != nil {
		return "", err
	}
	attemptID, err := LatestAttemptID(runDir, jobID)
	if err != nil || attemptID == "" {
		return jobDir, nil
	}
	return filepath.Join(jobDir, "attempts", attemptID), nil
}

func LatestAttemptID(runDir, jobID string) (string, error) {
	jobDir, err := SafeJoin(runDir, jobID)
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
		if !entry.IsDir() || !IsValidPathElement(entry.Name()) {
			continue
		}
		payload, decodeErr := decodeAttemptIDForRun(entry.Name(), runID)
		if decodeErr != nil {
			payload, decodeErr = DecodeAttemptID(entry.Name())
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

func decodeAttemptIDForRun(attemptID, runID string) (AttemptIDPayload, error) {
	prefix := "att_" + runID + "-"
	if !strings.HasPrefix(attemptID, prefix) {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	value := strings.TrimPrefix(attemptID, prefix)
	separator := strings.LastIndexByte(value, '-')
	if separator <= 0 || separator == len(value)-1 {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	number, err := strconv.Atoi(value[separator+1:])
	if err != nil || number < 0 {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	jobID := value[:separator]
	if !IsValidPathElement(runID) || !IsValidPathElement(jobID) {
		return AttemptIDPayload{}, fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	return AttemptIDPayload{RunID: runID, JobID: jobID, Number: number}, nil
}

func SpecificAttemptJobDir(runDir, jobID, attemptID string) (string, error) {
	jobDir, err := SafeJoin(runDir, jobID)
	if err != nil {
		return "", err
	}
	if !IsValidPathElement(attemptID) {
		return "", fmt.Errorf("invalid attempt ID %q", attemptID)
	}
	return filepath.Join(jobDir, "attempts", attemptID), nil
}
