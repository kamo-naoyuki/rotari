package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const diagnosisLogLimit = 12000

// diagnoseJobResult saves the rule-based analysis of a failed result from its
// latest attempt's output.
func diagnoseJobResult(runDir string, result model.JobResult) model.JobResult {
	if result.ExitCode == 0 || result.DiagnosisStatus != "" {
		return result
	}
	log, err := readDiagnosisLog(runDir, result.ID)
	return diagnose.AnalyzeResult(result, log, err)
}

func readDiagnosisLog(runDir, jobID string) (string, error) {
	if !state.IsValidPathElement(jobID) {
		return "", errors.New("the job ID is invalid, so its output could not be inspected")
	}
	jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
	if err != nil {
		return "", fmt.Errorf("the job directory could not be resolved: %w", err)
	}
	logs, err := readSeparateJobLogs(jobDir)
	if err != nil {
		return "", fmt.Errorf("the job logs could not be read: %w", err)
	}
	return diagnose.TailLog(logs, diagnosisLogLimit), nil
}

func readSeparateJobLogs(jobDir string) (string, error) {
	if data, err := os.ReadFile(filepath.Join(jobDir, "output")); err == nil {
		return string(data), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	var logs strings.Builder
	for _, stream := range []string{state.StdoutFileName, state.StderrFileName} {
		data, err := os.ReadFile(filepath.Join(jobDir, stream))
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if len(data) == 0 {
			continue
		}
		if logs.Len() > 0 {
			logs.WriteString("\n")
		}
		fmt.Fprintf(&logs, "--- %s ---\n", stream)
		logs.Write(data)
	}
	return logs.String(), nil
}
