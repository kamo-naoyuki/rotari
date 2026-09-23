package state

import (
	"fmt"
	"path/filepath"
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
