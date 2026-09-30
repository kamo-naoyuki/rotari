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
	Kind       EventKind        `json:"event"`
	OccurredAt string           `json:"occurred_at"`
	Project    string           `json:"project,omitempty"`
	RunID      string           `json:"run_id,omitempty"`
	RunName    string           `json:"run_name,omitempty"`
	RunStatus  string           `json:"run_status,omitempty"`
	Job        *model.JobSpec   `json:"job,omitempty"`
	Result     *model.JobResult `json:"result,omitempty"`
	StartedAt  string           `json:"started_at,omitempty"`
	FinishedAt string           `json:"finished_at,omitempty"`
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
	return Event{Kind: JobFinished, OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), Project: project, RunID: runID, RunName: runName, Job: &job, Result: &result, RunStatus: status}
}

func NewRunEvent(project string, summary model.RunSummary, occurredAt time.Time) Event {
	return Event{Kind: RunFinished, OccurredAt: occurredAt.UTC().Format(time.RFC3339Nano), Project: project, RunID: summary.RunID, RunName: summary.RunName, RunStatus: summary.Status, StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt}
}

func (settings ChannelSettings) Includes(event Event) bool {
	success := event.RunStatus == "success"
	if event.Kind == JobFinished {
		if success {
			return settings.JobSuccess
		}
		return settings.JobFailure
	}
	if success {
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
