package server

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// OpRun is the only request: the run a supervisor was started for.
const OpRun = "run"

func IsKnownOperation(op string) bool {
	return op == OpRun
}

type Request struct {
	Op               string                    `json:"op"`
	QueueName        string                    `json:"project_name,omitempty"`
	LocalConcurrency int                       `json:"local_concurrency,omitempty"`
	BatchMaxActive   int                       `json:"batch_max_active,omitempty"`
	ExecutorSettings executor.RunSettingsMap   `json:"executor_settings,omitempty"`
	Retry            int                       `json:"retry,omitempty"`
	RunName          string                    `json:"run_name,omitempty"`
	CWD              string                    `json:"cwd,omitempty"`
	ConfigPath       string                    `json:"config_path,omitempty"`
	FileConfig       *model.FileConfigSnapshot `json:"file_config,omitempty"`
	Async            bool                      `json:"async,omitempty"`
	DisconnectAction string                    `json:"disconnect_action,omitempty"`
	ClientSessionID  string                    `json:"client_session_id,omitempty"`
	Quiet            bool                      `json:"quiet,omitempty"`
	Executor         string                    `json:"executor,omitempty"`
	ExecutorOptions  []string                  `json:"executor_options,omitempty"`
	EnvMode          string                    `json:"env_mode,omitempty"`
	JobIDs           []string                  `json:"job_ids,omitempty"`
	Selection        string                    `json:"selection,omitempty"`
	// ScopeStage and ScopeMatrix narrow Selection to one stage or matrix, and
	// Filter narrows it further.
	ScopeStage   string           `json:"scope_stage,omitempty"`
	ScopeMatrix  string           `json:"scope_matrix,omitempty"`
	Filter       jobfilter.Filter `json:"filter"`
	SourceRunID  string           `json:"source_run_id,omitempty"`
	SourcePolicy string           `json:"source_policy,omitempty"`
	CopyAttempts bool             `json:"copy_attempts,omitempty"`
	CopyJobIDs   []string         `json:"copy_job_ids,omitempty"`
	PartialArray bool             `json:"partial_array,omitempty"`
	MatchBy      string           `json:"match_by,omitempty"`
	// IfRevision, when set, starts the run only if the project is still at
	// this revision; see project.Guard.
	IfRevision string `json:"if_revision,omitempty"`
}

type Response struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Protocol  int    `json:"protocol,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
	Accepted  bool   `json:"accepted,omitempty"`
	Notice    string `json:"notice,omitempty"`
	Progress  bool   `json:"progress,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	Completed int    `json:"completed,omitempty"`
	Total     int    `json:"total,omitempty"`
	Succeeded int    `json:"succeeded,omitempty"`
	Failed    int    `json:"failed,omitempty"`
	// RunID is the run accepted by either a synchronous or asynchronous request.
	RunID string `json:"run_id,omitempty"`
}
