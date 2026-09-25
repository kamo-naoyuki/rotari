package workflow

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// SourceRun is a finished run that a manifest is exported from or reconciled
// against.
type SourceRun struct {
	ID      string
	Queue   model.Queue
	Summary model.RunSummary
	CWD     string
	// JobTimestamps returns a job's submitted and finished times in the run.
	JobTimestamps func(jobID string) (submittedAt, finishedAt string)

	results map[string]model.JobResult
}

// CommandTimestamp is the time that orders snapshots of the same command
// across runs: the latest finish (or, for an unfinished job, submission) of
// its jobs, falling back to the run's finish.
func (run SourceRun) CommandTimestamp(command model.QueuedCommand) string {
	latest := ""
	for _, jobID := range commandLeafIDs(command) {
		submittedAt, finishedAt := run.JobTimestamps(jobID)
		timestamp := finishedAt
		if timestamp == "" {
			timestamp = submittedAt
		}
		if timestamp > latest {
			latest = timestamp
		}
	}
	if latest == "" {
		latest = run.Summary.FinishedAt
	}
	return latest
}

// newerSnapshot reports whether a command snapshot from run, with timestamp,
// replaces the one selected so far from selectedRunID.
func newerSnapshot(timestamp, runID, selectedTimestamp, selectedRunID string) bool {
	return timestamp > selectedTimestamp || timestamp == selectedTimestamp && runID > selectedRunID
}

// MergeRuns exports the listed runs of project as one manifest. Each command
// ID is described by its latest snapshot, in first-appearance order.
func MergeRuns(project string, runs []SourceRun) (Manifest, error) {
	type candidate struct {
		command   model.QueuedCommand
		results   []model.JobResult
		timestamp string
		runID     string
	}
	candidates := make(map[string]candidate)
	order := make([]string, 0)
	for _, run := range runs {
		results := model.ResultsByID(run.Summary.Results)
		for _, command := range run.Queue.Commands {
			next := candidate{command: command, results: commandResults(command, results), timestamp: run.CommandTimestamp(command), runID: run.ID}
			previous, exists := candidates[command.ID]
			if !exists {
				order = append(order, command.ID)
			}
			if !exists || newerSnapshot(next.timestamp, next.runID, previous.timestamp, previous.runID) {
				candidates[command.ID] = next
			}
		}
	}
	queue := model.Queue{}
	summary := model.RunSummary{}
	for _, id := range order {
		selected := candidates[id]
		queue.Commands = append(queue.Commands, selected.command)
		summary.Results = append(summary.Results, selected.results...)
	}
	if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
		return Manifest{}, fmt.Errorf("cannot merge runs: %w", err)
	}
	runIDs := make([]string, len(runs))
	for index, run := range runs {
		runIDs[index] = run.ID
	}
	return FromRun(queue, summary, Source{Project: project, RunIDs: runIDs})
}

func commandResults(command model.QueuedCommand, results map[string]model.JobResult) []model.JobResult {
	selected := make([]model.JobResult, 0)
	for _, jobID := range commandLeafIDs(command) {
		if result, ok := results[jobID]; ok {
			selected = append(selected, result)
		}
	}
	return selected
}

// FlattenQueueDefaults copies the queue's default executor and options into
// each command that does not set its own, so commands compare and export
// independently of the queue they came from.
func FlattenQueueDefaults(queue model.Queue) model.Queue {
	for index := range queue.Commands {
		if queue.Commands[index].Executor == "" {
			queue.Commands[index].Executor = queue.DefaultExecutor
		}
		if len(queue.Commands[index].ExecutorOptions) == 0 {
			queue.Commands[index].ExecutorOptions = append([]string(nil), queue.DefaultExecutorOptions...)
		}
	}
	queue.DefaultExecutor = ""
	queue.DefaultExecutorOptions = nil
	return queue
}
