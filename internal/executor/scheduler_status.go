package executor

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

type SchedulerStatus struct {
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
}

func WriteSchedulerStatus(store state.Store, jobDir, value string, now time.Time) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return
	}
	path, err := state.ValidatedStateFile(jobDir, "scheduler_status.json")
	if err != nil {
		return
	}
	_ = store.WriteJSON(path, SchedulerStatus{State: value, UpdatedAt: now.UTC().Format(time.RFC3339)})
}

func LoadSchedulerStatus(store state.Store, jobDir string) string {
	path, err := state.ValidatedStateFile(jobDir, "scheduler_status.json")
	if err != nil {
		return ""
	}
	var status SchedulerStatus
	if store.ReadJSON(path, &status) != nil {
		return ""
	}
	return status.State
}

func SchedulerStateTerminal(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "finished", "completed", "complete", "success", "succeeded", "failed", "cancelled", "canceled", "timeout", "out_of_memory", "oom", "unknown":
		return true
	default:
		return false
	}
}

func SchedulerStateExitCode(value string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "completed", "complete", "success", "succeeded", "finished":
		return 0, true
	case "failed", "cancelled", "canceled", "timeout", "out_of_memory", "oom", "unknown":
		return 1, true
	default:
		return 0, false
	}
}

func ResolveTerminalExitCode(store state.Store, jobDir string) (int, bool) {
	if path, err := state.ValidatedStateFile(jobDir, "status"); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			if exitCode, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				return exitCode, true
			}
		}
	}
	if path, err := state.ValidatedStateFile(jobDir, "status.json"); err == nil {
		if status, ok := LoadWrapperStatus(store, path); ok && (status.FinishedAt != "" || SchedulerStateTerminal(status.Phase)) {
			return status.ExitCode, true
		}
	}
	return SchedulerStateExitCode(LoadSchedulerStatus(store, jobDir))
}
