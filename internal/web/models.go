package web

import "github.com/kamo-naoyuki/rotari/internal/model"

type EnvironmentDefinition struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Set         bool   `json:"set,omitempty"`
	CLIDefault  bool   `json:"cli_default"`
	Job         bool   `json:"job"`
	Array       bool   `json:"array"`
	Description string `json:"description"`
}

type Run struct {
	model.RunSummary
	Jobs     []Job            `json:"jobs"`
	CWD      string           `json:"cwd,omitempty"`
	Context  model.RunContext `json:"context,omitempty"`
	Timeline []TimelinePoint  `json:"timeline,omitempty"`
	Running  bool             `json:"running"`
}

type Job struct {
	ID               string           `json:"id"`
	AttemptID        string           `json:"attempt_id,omitempty"`
	Attempts         []Attempt        `json:"attempts,omitempty"`
	ArrayTaskID      *int             `json:"array_task_id,omitempty"`
	ArrayFirst       int              `json:"array_first,omitempty"`
	ArrayLast        int              `json:"array_last,omitempty"`
	Name             string           `json:"name,omitempty"`
	Stage            string           `json:"stage,omitempty"`
	Command          []string         `json:"command"`
	WorkingDirectory string           `json:"working_directory,omitempty"`
	Executor         string           `json:"executor,omitempty"`
	ExecutorOptions  []string         `json:"executor_options,omitempty"`
	DependsOn        []string         `json:"depends_on,omitempty"`
	Result           *model.JobResult `json:"result,omitempty"`
	Origin           *model.JobOrigin `json:"origin,omitempty"`
	AttemptDir       string           `json:"-"`
	SubmittedAt      string           `json:"submitted_at,omitempty"`
	FinishedAt       string           `json:"finished_at,omitempty"`
	SchedulerState   string           `json:"scheduler_state,omitempty"`
}

type Attempt struct {
	ID             string           `json:"id"`
	Result         *model.JobResult `json:"result,omitempty"`
	SubmittedAt    string           `json:"submitted_at,omitempty"`
	FinishedAt     string           `json:"finished_at,omitempty"`
	SchedulerState string           `json:"scheduler_state,omitempty"`
}

type QueueState struct {
	QueueName       string      `json:"project_name"`
	ConfigPath      string      `json:"config_path,omitempty"`
	Queue           model.Queue `json:"queue"`
	Runs            []Run       `json:"runs"`
	RunnerPID       int         `json:"runner_pid,omitempty"`
	RunningRunID    string      `json:"running_run_id,omitempty"`
	RunnerHost      string      `json:"runner_host,omitempty"`
	RunnerStartedAt string      `json:"runner_started_at,omitempty"`
}

type ServerState struct {
	PID           int  `json:"pid,omitempty"`
	PIDFileExists bool `json:"pid_file_exists"`
	SocketExists  bool `json:"socket_exists"`
}

type ConfigFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type State struct {
	BaseDir      string                  `json:"base_dir"`
	ConfigPath   string                  `json:"config_path,omitempty"`
	Queues       []QueueState            `json:"projects"`
	Server       ServerState             `json:"server"`
	Environments []EnvironmentDefinition `json:"environments"`
	UpdatedAt    string                  `json:"updated_at"`
}
