package executor

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// WrapperStatus is the contents of status.json, written by StatusWrapperScript
// as a job runs and read back by both the local and scheduler executors (and
// by cmd/rotari's show/web/run-summary code, which polls it directly).
type WrapperStatus struct {
	Phase      string   `json:"phase"`
	ExitCode   int      `json:"exit_code,omitempty"`
	Error      string   `json:"error,omitempty"`
	Hosts      []string `json:"hosts,omitempty"`
	StartedAt  string   `json:"started_at,omitempty"`
	FinishedAt string   `json:"finished_at,omitempty"`
}

func LoadWrapperStatus(store state.Store, path string) (WrapperStatus, bool) {
	var status WrapperStatus
	if err := store.ReadJSON(path, &status); err != nil {
		return WrapperStatus{}, false
	}
	return status, true
}

func jobResultFromStatus(jobID string, command []string, status WrapperStatus) model.JobResult {
	return model.JobResult{ID: jobID, Command: command, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}
}
