package state

import (
	"encoding/json"
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
	// codeql[go/path-injection]: jobDir is produced by SafeJoin and attempts is a fixed directory.
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

func ReadJobTimestamp(runDir, jobID, name string) string {
	if name != "submitted_at" && name != "finished_at" {
		return ""
	}
	jobDir, err := LatestAttemptJobDir(runDir, jobID)
	if err != nil {
		return ""
	}
	path, err := ValidatedStateFile(jobDir, name)
	if err == nil {
		// codeql[go/path-injection]: path is restricted by ValidatedStateFile to a fixed timestamp file.
		if data, err := os.ReadFile(path); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	metadataPath, err := ValidatedStateFile(jobDir, "job.json")
	if err == nil {
		// codeql[go/path-injection]: metadataPath is restricted by ValidatedStateFile to job.json.
		data, err := os.ReadFile(metadataPath)
		if err == nil {
			var metadata struct {
				SubmittedAt string `json:"submitted_at"`
			}
			if err := json.Unmarshal(data, &metadata); err == nil && metadata.SubmittedAt != "" {
				if name == "submitted_at" {
					return metadata.SubmittedAt
				}
			}
		}
	}
	statusPath, err := ValidatedStateFile(jobDir, "status.json")
	if err == nil {
		// codeql[go/path-injection]: statusPath is restricted by ValidatedStateFile to status.json.
		data, err := os.ReadFile(statusPath)
		if err == nil {
			var status struct {
				FinishedAt string `json:"finished_at"`
			}
			if err := json.Unmarshal(data, &status); err == nil && status.FinishedAt != "" {
				if name == "finished_at" {
					return status.FinishedAt
				}
			}
		}
	}
	return ""
}

func ReadAttemptTimestamp(jobDir, name string) string {
	path, err := ValidatedStateFile(jobDir, name)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func LoadLocalJobResult(jobDir string, job model.JobSpec) (model.JobResult, bool) {
	finishedPath, err := ValidatedStateFile(jobDir, "finished_at")
	if err != nil {
		return model.JobResult{}, false
	}
	if _, err := os.Stat(finishedPath); err != nil {
		return model.JobResult{}, false
	}
	path, err := ValidatedStateFile(jobDir, "status")
	if err != nil {
		return model.JobResult{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return model.JobResult{}, false
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return model.JobResult{}, false
	}
	return model.JobResult{ID: job.ID, Command: job.Command, ExitCode: exitCode}, true
}
