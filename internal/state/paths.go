package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	StdoutFileName = "stdout"
	StderrFileName = "stderr"
)

func IsValidPathElement(value string) bool {
	if value == "" || value == "." || value == ".." || filepath.IsAbs(value) {
		return false
	}
	if strings.ContainsAny(value, `/\\`) {
		return false
	}
	return filepath.Base(value) == value
}

func ValidateStatePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty state path")
	}
	for _, element := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if element == "." || element == ".." {
			return fmt.Errorf("invalid state path %q", path)
		}
	}
	return nil
}

func SafeJoin(basePath, element string) (string, error) {
	if !IsValidPathElement(element) {
		return "", fmt.Errorf("invalid path element %q", element)
	}
	root := filepath.Clean(basePath)
	// NOSONAR: element is validated as a single path element before it reaches this join.
	joined := filepath.Join(root, element)
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path element %q", element)
	}
	return joined, nil
}

func ValidatedStateFile(basePath, fileName string) (string, error) {
	switch fileName {
	case "commands.json", "summary.json", "context.json", RunClientStatusFileName, "output", StdoutFileName, StderrFileName, "scheduler_status.json",
		"status.json", "status", "submitted_at", "finished_at", "command.json", "job.json",
		"pid", "cancelled", "name":
		return SafeJoin(basePath, fileName)
	default:
		return "", fmt.Errorf("invalid state file name %q", fileName)
	}
}

func RequireRunStateFile(runDir, name, runID string) error {
	path, err := ValidatedStateFile(runDir, name)
	if err != nil {
		return err
	}
	// codeql[go/path-injection]: path is restricted by ValidatedStateFile to a fixed state file name.
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("run %q is missing %s: %w", runID, name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("run %q %s is not a regular file", runID, name)
	}
	return nil
}

func ValidateRunDirectory(runsDir, runID string, requireCommands bool) (string, error) {
	runDir, err := SafeJoin(runsDir, runID)
	if err != nil {
		return "", err
	}
	// codeql[go/path-injection]: runDir is produced by SafeJoin after validating runID.
	info, err := os.Stat(runDir)
	if err != nil {
		return "", fmt.Errorf("run %q directory is missing: %w", runID, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("run %q path is not a directory", runID)
	}
	if err := RequireRunStateFile(runDir, "context.json", runID); err != nil {
		return "", err
	}
	if requireCommands {
		if err := RequireRunStateFile(runDir, "commands.json", runID); err != nil {
			return "", err
		}
	}
	return runDir, nil
}

// LatestAttemptDirs returns, sorted, the latest attempt directory of each job
// of runDir that has a command snapshot: JOB/attempts/ATTEMPT, or JOB itself
// in a run written before attempts had directories of their own.
func LatestAttemptDirs(runDir string) ([]string, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	attemptDirs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !IsValidPathElement(entry.Name()) {
			continue
		}
		attemptDir, err := LatestAttemptJobDir(runDir, entry.Name())
		if err != nil {
			continue
		}
		if err := RequireRunStateFile(attemptDir, "command.json", entry.Name()); err != nil {
			continue
		}
		attemptDirs = append(attemptDirs, attemptDir)
	}
	sort.Strings(attemptDirs)
	return attemptDirs, nil
}
