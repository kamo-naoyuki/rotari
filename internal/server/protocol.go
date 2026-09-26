package server

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
)

const (
	OpPing     = "ping"
	OpShutdown = "shutdown"
	OpRun      = "run"
	OpCancel   = "cancel"
	OpSuspend  = "suspend"
	OpResume   = "resume"
)

func IsKnownOperation(op string) bool {
	switch op {
	case OpPing, OpShutdown, OpRun, OpCancel, OpSuspend, OpResume:
		return true
	default:
		return false
	}
}

type Request struct {
	Op               string                  `json:"op"`
	QueueName        string                  `json:"project_name,omitempty"`
	LocalConcurrency int                     `json:"local_concurrency,omitempty"`
	BatchMaxActive   int                     `json:"batch_max_active,omitempty"`
	ExecutorSettings executor.RunSettingsMap `json:"executor_settings,omitempty"`
	Retry            int                     `json:"retry,omitempty"`
	RunName          string                  `json:"run_name,omitempty"`
	CWD              string                  `json:"cwd,omitempty"`
	Wait             bool                    `json:"wait,omitempty"`
	Async            bool                    `json:"async,omitempty"`
	Quiet            bool                    `json:"quiet,omitempty"`
	Executor         string                  `json:"executor,omitempty"`
	ExecutorOptions  []string                `json:"executor_options,omitempty"`
	JobIDs           []string                `json:"job_ids,omitempty"`
	// RunID, for cancel, suspend, and resume, is the run the selection
	// names; the project's active run must be it.
	RunID     string `json:"run_id,omitempty"`
	Selection string `json:"selection,omitempty"`
	// ScopeStage and ScopeMatrix narrow Selection to one stage or matrix.
	ScopeStage   string `json:"scope_stage,omitempty"`
	ScopeMatrix  string `json:"scope_matrix,omitempty"`
	SourceRunID  string `json:"source_run_id,omitempty"`
	PartialArray bool   `json:"partial_array,omitempty"`
}

type Response struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Protocol  int    `json:"protocol,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
	Progress  bool   `json:"progress,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	Completed int    `json:"completed,omitempty"`
	Total     int    `json:"total,omitempty"`
	Succeeded int    `json:"succeeded,omitempty"`
	Failed    int    `json:"failed,omitempty"`
}
