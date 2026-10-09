package notification

import (
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type EventKind string

const (
	JobFinished EventKind = "job.finished"
	RunFinished EventKind = "run.finished"
)

type Event struct {
	Kind              EventKind        `json:"event"`
	OccurredAt        string           `json:"occurred_at"`
	Project           string           `json:"project,omitempty"`
	RunID             string           `json:"run_id,omitempty"`
	RunName           string           `json:"run_name,omitempty"`
	RunStatus         string           `json:"run_status,omitempty"`
	JobStatus         string           `json:"job_status,omitempty"`
	ExitCode          int              `json:"exit_code,omitempty"`
	Successes         int              `json:"success_count,omitempty"`
	Failures          int              `json:"failure_count,omitempty"`
	DiagnosisOutdated bool             `json:"diagnosis_outdated,omitempty"`
	Job               *model.JobSpec   `json:"job,omitempty"`
	Result            *model.JobResult `json:"result,omitempty"`
	StartedAt         string           `json:"started_at,omitempty"`
	FinishedAt        string           `json:"finished_at,omitempty"`
}

type Batch struct {
	SchemaVersion int     `json:"schema_version"`
	CreatedAt     string  `json:"created_at"`
	Project       string  `json:"project"`
	RunID         string  `json:"run_id"`
	RunName       string  `json:"run_name,omitempty"`
	Events        []Event `json:"events"`
	OmittedJobs   int     `json:"omitted_jobs,omitempty"`
}

func NewJobEvent(project, runID, runName string, job model.JobSpec, result model.JobResult, occurredAt time.Time) Event {
	status := "success"
	if result.ExitCode != 0 {
		status = "failed"
	}
	return Event{Kind: JobFinished, OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), Project: project, RunID: runID, RunName: runName, Job: &job, Result: &result, JobStatus: status}
}

func (event *Event) SetTimestamps(startedAt, finishedAt string) {
	event.StartedAt = startedAt
	event.FinishedAt = finishedAt
}

func (event *Event) SetDiagnosisOutdated(outdated bool) {
	event.DiagnosisOutdated = outdated
}

func NewRunEvent(project string, summary model.RunSummary, occurredAt time.Time) Event {
	event := Event{Kind: RunFinished, OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), Project: project, RunID: summary.RunID, RunName: summary.RunName, RunStatus: summary.Status, ExitCode: summary.ExitCode, StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt}
	for _, result := range summary.Results {
		if result.ExitCode == 0 {
			event.Successes++
		} else {
			event.Failures++
		}
	}
	return event
}

func (settings ChannelSettings) Includes(event Event) bool {
	if event.Kind == JobFinished {
		if event.JobStatus == model.StatusSuccess {
			return settings.JobSuccess
		}
		return settings.JobFailure
	}
	// A run's status text is "finished" or "failed", never "success"; its
	// exit code is its outcome, and any other run counts as a failure.
	if event.ExitCode == 0 {
		return settings.RunSuccess
	}
	return settings.RunFailure
}

func NewBatch(events []Event, settings ChannelSettings, createdAt time.Time) Batch {
	batch := Batch{SchemaVersion: 1, CreatedAt: createdAt.UTC().Format(time.RFC3339Nano)}
	if len(events) > 0 {
		batch.Project, batch.RunID, batch.RunName = events[0].Project, events[0].RunID, events[0].RunName
	}
	jobs := 0
	for _, event := range events {
		if !settings.Includes(event) {
			continue
		}
		if event.Kind == JobFinished {
			jobs++
			if jobs > settings.MaxJobs {
				batch.OmittedJobs++
				continue
			}
		}
		batch.Events = append(batch.Events, event)
	}
	return batch
}

func (batch Batch) Empty() bool {
	return len(batch.Events) == 0
}
