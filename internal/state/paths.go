package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	case "commands.json", "summary.json", "context.json", "output", "scheduler_status.json",
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

func ListRunJobDirs(runDir string) ([]string, error) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}
	jobDirs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobDir, err := SafeJoin(runDir, entry.Name())
		if err != nil {
			continue
		}
		if err := RequireRunStateFile(jobDir, "command.json", entry.Name()); err != nil {
			continue
		}
		jobDirs = append(jobDirs, jobDir)
	}
	sort.Strings(jobDirs)
	return jobDirs, nil
}
