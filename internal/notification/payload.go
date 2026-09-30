package notification

import (
	"fmt"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type Field struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type PayloadEvent struct {
	Event  EventKind `json:"event"`
	Fields []Field   `json:"fields"`
}

type Payload struct {
	SchemaVersion int            `json:"schema_version"`
	CreatedAt     string         `json:"created_at"`
	Events        []PayloadEvent `json:"events"`
	OmittedJobs   int            `json:"omitted_jobs,omitempty"`
}

func NewPayload(batch Batch, selected []string) Payload {
	payload := Payload{SchemaVersion: batch.SchemaVersion, CreatedAt: batch.CreatedAt, OmittedJobs: batch.OmittedJobs}
	successes, failures := 0, 0
	for _, event := range batch.Events {
		if event.Kind != JobFinished {
			if event.Kind == RunFinished {
				successes, failures = event.Successes, event.Failures
			}
			continue
		}
		if event.JobStatus == model.StatusSuccess {
			successes++
		} else {
			failures++
		}
	}
	for _, event := range batch.Events {
		fields := make([]Field, 0, len(selected))
		for _, name := range selected {
			fields = append(fields, eventFields(event, name, successes, failures)...)
		}
		payload.Events = append(payload.Events, PayloadEvent{Event: event.Kind, Fields: fields})
	}
	return payload
}

func eventFields(event Event, name string, successes, failures int) []Field {
	add := func(value any, present bool) []Field {
		if !present {
			return nil
		}
		return []Field{{Name: name, Value: value}}
	}
	if name == "exit_code" && event.Kind == RunFinished {
		return add(event.ExitCode, true)
	}
	switch name {
	case "project":
		return add(event.Project, event.Project != "")
	case "run_id":
		return add(event.RunID, event.RunID != "")
	case "run_name":
		return add(event.RunName, event.RunName != "")
	case "run_status":
		return add(event.RunStatus, event.RunStatus != "")
	case "success_count":
		return add(successes, event.Kind == RunFinished)
	case "failure_count":
		return add(failures, event.Kind == RunFinished)
	case "total_count":
		return add(successes+failures, event.Kind == RunFinished)
	case "started_at":
		return add(event.StartedAt, event.StartedAt != "")
	case "finished_at":
		return add(event.FinishedAt, event.FinishedAt != "")
	case "duration":
		return add(duration(event.StartedAt, event.FinishedAt))
	}
	if event.Job == nil || event.Result == nil {
		return nil
	}
	job, result := *event.Job, *event.Result
	switch name {
	case "job_id":
		return add(job.ID, job.ID != "")
	case "job_name":
		return add(job.Name, job.Name != "")
	case "stage":
		return add(job.Stage, job.Stage != "")
	case "array_task_id":
		if job.ArrayTaskID == nil {
			return nil
		}
		return add(*job.ArrayTaskID, true)
	case "attempt_id":
		value := result.AttemptID
		if value == "" {
			value = job.AttemptID
		}
		return add(value, value != "")
	case "job_status":
		return add(event.JobStatus, event.JobStatus != "")
	case "exit_code":
		return add(result.ExitCode, true)
	case "error":
		return add(result.Error, result.Error != "")
	case "executor":
		return add(job.Executor, job.Executor != "")
	case "hosts":
		return add(result.Hosts, len(result.Hosts) > 0)
	case "working_directory":
		return add(job.WorkingDirectory, job.WorkingDirectory != "")
	case "command":
		command := result.Command
		if len(command) == 0 {
			command = job.Command
		}
		return add(command, len(command) > 0)
	case "diagnosis_status":
		return add(result.DiagnosisStatus, result.DiagnosisStatus != "")
	case "diagnosis_rules":
		return add(result.DiagnosisRules, result.DiagnosisRules != "")
	case "diagnosis_outdated":
		return add(event.DiagnosisOutdated, result.DiagnosisStatus != "")
	case "diagnosis_name", "diagnosis_evidence", "diagnosis_suggestion":
		return diagnosisFields(result.Diagnoses, name)
	}
	return nil
}

func diagnosisFields(diagnoses []model.RuleDiagnosis, name string) []Field {
	fields := make([]Field, 0, len(diagnoses))
	for _, diagnosis := range diagnoses {
		value := ""
		switch name {
		case "diagnosis_name":
			value = diagnosis.Name
		case "diagnosis_evidence":
			value = diagnosis.Evidence
		case "diagnosis_suggestion":
			value = diagnosis.Suggestion
		}
		if value != "" {
			fields = append(fields, Field{Name: name, Value: value})
		}
	}
	return fields
}

func duration(startedAt, finishedAt string) (string, bool) {
	started, startErr := time.Parse(time.RFC3339Nano, startedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, finishedAt)
	if startErr != nil || finishErr != nil || finished.Before(started) {
		return "", false
	}
	return finished.Sub(started).String(), true
}

func FormatValue(value any) string {
	switch typed := value.(type) {
	case []string:
		return strings.Join(typed, ", ")
	default:
		return fmt.Sprint(value)
	}
}
