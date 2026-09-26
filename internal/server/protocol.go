package server

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

const (
	OpPing     = "ping"
	OpShutdown = "shutdown"
	OpRun      = "run"
	OpSubmit   = "submit"
	OpCancel   = "cancel"
	OpSuspend  = "suspend"
	OpResume   = "resume"
	OpCopy     = "copy"
	OpChange   = "change"
	OpRemove   = "remove"
	OpClear    = "clear"
)

func IsKnownOperation(op string) bool {
	switch op {
	case OpPing, OpShutdown, OpRun, OpSubmit, OpCancel, OpSuspend, OpResume, OpCopy, OpChange, OpRemove, OpClear:
		return true
	default:
		return false
	}
}

type Request struct {
	Op                string                  `json:"op"`
	QueueName         string                  `json:"project_name,omitempty"`
	Command           []string                `json:"command,omitempty"`
	LocalConcurrency  int                     `json:"local_concurrency,omitempty"`
	BatchMaxActive    int                     `json:"batch_max_active,omitempty"`
	ExecutorSettings  executor.RunSettingsMap `json:"executor_settings,omitempty"`
	Retry             int                     `json:"retry,omitempty"`
	RunName           string                  `json:"run_name,omitempty"`
	CWD               string                  `json:"cwd,omitempty"`
	Wait              bool                    `json:"wait,omitempty"`
	Async             bool                    `json:"async,omitempty"`
	Quiet             bool                    `json:"quiet,omitempty"`
	Executor          string                  `json:"executor,omitempty"`
	ExecutorOptions   []string                `json:"executor_options,omitempty"`
	WorkingDirectory  string                  `json:"working_directory,omitempty"`
	Environment       []string                `json:"environment,omitempty"`
	JobIDs            []string                `json:"job_ids,omitempty"`
	Selection         string                  `json:"selection,omitempty"`
	SourceRunID       string                  `json:"source_run_id,omitempty"`
	PartialArray      bool                    `json:"partial_array,omitempty"`
	JobName           string                  `json:"job_name,omitempty"`
	Stage             string                  `json:"stage,omitempty"`
	DependsOn         []string                `json:"depends_on,omitempty"`
	DependsOnFinished []string                `json:"depends_on_finished,omitempty"`
	Timeout           string                  `json:"timeout,omitempty"`
	// JobRetry is a submitted job's own retry limit; Retry is the run's.
	JobRetry *int `json:"job_retry,omitempty"`
	// RetryDelay, RetryBackoff, and RetryMaxDelay space out the job's
	// retries.
	RetryDelay    string           `json:"retry_delay,omitempty"`
	RetryBackoff  float64          `json:"retry_backoff,omitempty"`
	RetryMaxDelay string           `json:"retry_max_delay,omitempty"`
	Array         *model.ArraySpec `json:"array,omitempty"`
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
