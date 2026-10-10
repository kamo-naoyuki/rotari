package web

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
)

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
	LineageSummary runlineage.RunSummary `json:"lineage_summary,omitempty"`
	Jobs           []Job                 `json:"jobs"`
	Lifecycle      string                `json:"lifecycle,omitempty"`
	ClientStatus   model.RunClientStatus `json:"client_status"`
	ClientLabel    string                `json:"client_label,omitempty"`
	CWD            string                `json:"cwd,omitempty"`
	Context        model.RunContext      `json:"context,omitempty"`
	// Sources is the code the run's executed jobs ran from, and SourceLabels
	// each as model.SourceLabel describes it for `show`.
	Sources      []model.SourceRevision `json:"sources,omitempty"`
	SourceLabels []string               `json:"source_labels,omitempty"`
	// Notes are the run's notes, oldest first, including those on its jobs.
	// The run page shows the run's own notes in its report, and a job's
	// behind the job's Notes button, from Job.NoteLabels.
	Notes    []model.RunNote `json:"notes,omitempty"`
	Timeline []TimelinePoint `json:"timeline,omitempty"`
	Running  bool            `json:"running"`
	// Unreadable, when set, is why the run's files cannot be read: they come
	// from a newer rotari. The run then has status "unreadable" and no jobs.
	Unreadable string `json:"unreadable,omitempty"`
}

type Job struct {
	ID                string    `json:"id"`
	AttemptID         string    `json:"attempt_id,omitempty"`
	Attempts          []Attempt `json:"attempts,omitempty"`
	ArrayTaskID       *int      `json:"array_task_id,omitempty"`
	ArrayFirst        int       `json:"array_first,omitempty"`
	ArrayLast         int       `json:"array_last,omitempty"`
	Name              string    `json:"name,omitempty"`
	Stage             string    `json:"stage,omitempty"`
	Command           []string  `json:"command"`
	WorkingDirectory  string    `json:"working_directory,omitempty"`
	Executor          string    `json:"executor,omitempty"`
	ExecutorOptions   []string  `json:"executor_options,omitempty"`
	LogMode           string    `json:"log_mode,omitempty"`
	DependsOn         []string  `json:"depends_on,omitempty"`
	DependsOnFinished []string  `json:"depends_on_finished,omitempty"`
	// Matrix places a matrix member in its group's grid.
	Matrix          *Matrix          `json:"matrix,omitempty"`
	Result          *model.JobResult `json:"result,omitempty"`
	Origin          *model.JobOrigin `json:"origin,omitempty"`
	Carried         bool             `json:"carried,omitempty"`
	ExecutionStatus string           `json:"execution_status"`
	AttemptDir      string           `json:"-"`
	// Environment is the job's own NAME=VALUE environment, matrix values
	// included, for the run report. The Web API leaves it out: values given
	// with --env may be secrets.
	Environment    []string `json:"-"`
	SubmittedAt    string   `json:"submitted_at,omitempty"`
	FinishedAt     string   `json:"finished_at,omitempty"`
	SchedulerState string   `json:"scheduler_state,omitempty"`
	Final          bool     `json:"final,omitempty"`
	// DiagnosisOutdated reports that Result's saved rule-based analysis was
	// produced by earlier diagnosis rules.
	DiagnosisOutdated bool `json:"diagnosis_outdated,omitempty"`
	// NoteLabels are the run's notes on this job, oldest first, each as
	// model.FormatRunNote describes it without the job, naming the attempt
	// when it is not the job's latest.
	NoteLabels []string `json:"note_labels,omitempty"`
	// lineageStatus is the job's runview.LineageStatus for the run summary.
	lineageStatus string
}

type Attempt struct {
	ID              string           `json:"id"`
	Result          *model.JobResult `json:"result,omitempty"`
	ExecutionStatus string           `json:"execution_status"`
	SubmittedAt     string           `json:"submitted_at,omitempty"`
	FinishedAt      string           `json:"finished_at,omitempty"`
	SchedulerState  string           `json:"scheduler_state,omitempty"`
}

type QueueState struct {
	QueueName       string               `json:"project_name"`
	ConfigPath      string               `json:"config_path,omitempty"`
	ConfigSources   []model.ConfigSource `json:"config_sources,omitempty"`
	Queue           model.Queue          `json:"queue"`
	Runs            []Run                `json:"runs"`
	RunCount        int                  `json:"run_count"`
	Revision        string               `json:"revision,omitempty"`
	RunnerPID       int                  `json:"runner_pid,omitempty"`
	RunningRunID    string               `json:"running_run_id,omitempty"`
	RunnerHost      string               `json:"runner_host,omitempty"`
	RunnerStartedAt string               `json:"runner_started_at,omitempty"`
	// Server is the persisted record of the project's supervisor, which
	// exists only while a run is starting or active.
	Server ServerState `json:"server"`
}

type ServerState struct {
	PID           int  `json:"pid,omitempty"`
	PIDFileExists bool `json:"pid_file_exists"`
}

type ConfigFile struct {
	Scope   string `json:"scope,omitempty"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

type State struct {
	BaseDir       string                  `json:"base_dir"`
	ConfigPath    string                  `json:"config_path,omitempty"`
	ConfigSources []model.ConfigSource    `json:"config_sources,omitempty"`
	Queues        []QueueState            `json:"projects"`
	Environments  []EnvironmentDefinition `json:"environments"`
	UpdatedAt     string                  `json:"updated_at"`
}

// Matrix is the part of a job's matrix provenance the Web UI needs to draw a
// grid. It omits the base environment, which may hold secrets.
type Matrix struct {
	GroupID    string                  `json:"group_id"`
	BaseName   string                  `json:"base_name,omitempty"`
	Dimensions []model.MatrixDimension `json:"dimensions"`
	Values     []model.MatrixValue     `json:"values"`
}
