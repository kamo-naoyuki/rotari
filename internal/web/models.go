package web

import "github.com/kamo-naoyuki/rotari/internal/model"

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
