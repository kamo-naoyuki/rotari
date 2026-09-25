package web

import (
	"fmt"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type QueueLoader struct {
	ProjectName string
	Queue       func() (model.Queue, error)
	Lock        func() (model.LockInfo, error)
	Runs        func() ([]string, error)
	Summary     func(runID string) (model.RunSummary, error)
	Jobs        func(runID string, summary model.RunSummary) ([]Job, error)
	Context     func(runID string) (model.RunContext, error)
	Samples     func(runID string) []model.LoadSample
}

func LoadQueueState(loader QueueLoader) (QueueState, error) {
	queue, err := loader.Queue()
	if err != nil {
		return QueueState{}, err
	}
	state := QueueState{QueueName: loader.ProjectName, Queue: queue, Runs: make([]Run, 0)}
	runningStartedAt := ""
	if lock, lockErr := loader.Lock(); lockErr == nil {
		state.RunningRunID = lock.RunID
		state.RunnerPID = lock.PID
		state.RunnerHost = lock.Host
		state.RunnerStartedAt = lock.StartedAt
		runningStartedAt = lock.StartedAt
	}
	runIDs, err := loader.Runs()
	if err != nil {
		return QueueState{}, err
	}
	for _, runID := range runIDs {
		summary, summaryErr := loader.Summary(runID)
		if summaryErr != nil {
			summary = model.RunSummary{RunID: runID, Status: "running", StartedAt: runningStartedAt}
		}
		if summary.RunID == "" {
			summary.RunID = runID
		}
		jobs, err := loader.Jobs(runID, summary)
		if err != nil {
			return QueueState{}, err
		}
		context, contextErr := loader.Context(runID)
		if contextErr != nil {
			context = model.RunContext{}
		}
		context.LoadSamples = loader.Samples(runID)
		state.Runs = append(state.Runs, Run{RunSummary: summary, Jobs: jobs, CWD: context.CWD, Context: context, Timeline: buildTimeline(summary, jobs), Running: runID == state.RunningRunID})
	}
	sort.Slice(state.Runs, func(i, j int) bool { return state.Runs[i].RunID > state.Runs[j].RunID })
	return state, nil
}

// LoadJobs projects a run's jobs for the Web UI. Each job's result follows
// the shared jobstatus fallback chain. A non-empty selectedAttemptID shows
// that attempt, instead of the latest one, for its job.
func LoadJobs(store state.Store, runDir string, commands model.Queue, summary model.RunSummary, selectedAttemptID string) ([]Job, error) {
	results := model.ResultsByID(summary.Results)
	origins := model.QueueOriginsByJobID(commands)
	taskJobs := model.QueueToJobs(commands.Commands)
	selectedJobID := ""
	if selectedAttemptID != "" {
		if payload, err := state.DecodeAttemptID(selectedAttemptID); err == nil {
			selectedJobID = payload.JobID
		}
	}
	jobs := make([]Job, 0, len(taskJobs))
	for _, jobSpec := range taskJobs {
		selected := jobSpec.ID == selectedJobID
		jobDir, pathErr := state.LatestAttemptJobDir(runDir, jobSpec.ID)
		if selected {
			jobDir, pathErr = state.SpecificAttemptJobDir(runDir, jobSpec.ID, selectedAttemptID)
		}
		if pathErr != nil {
			return nil, fmt.Errorf("invalid job ID %q: %w", jobSpec.ID, pathErr)
		}
		origin := origins[jobSpec.ID]
		submittedAt, finishedAt := jobstatus.Timestamps(runDir, jobSpec.ID, origin)
		summaryResult, hasSummary := results[jobSpec.ID]
		attemptID := jobSpec.AttemptID
		if attemptID == "" && hasSummary {
			attemptID = summaryResult.AttemptID
		}
		attempt := jobstatus.ReadAttempt(store, jobDir)
		job := Job{ID: jobSpec.ID, AttemptID: attemptID, AttemptDir: jobDir, Name: jobSpec.Name, Stage: jobSpec.Stage, Command: jobSpec.Command, WorkingDirectory: jobSpec.WorkingDirectory, Executor: jobSpec.Executor, ExecutorOptions: jobSpec.ExecutorOptions, DependsOn: jobSpec.DependsOn, Origin: origin, ArrayTaskID: jobSpec.ArrayTaskID, ArrayFirst: jobSpec.ArrayFirst, ArrayLast: jobSpec.ArrayLast, SubmittedAt: submittedAt, FinishedAt: finishedAt, SchedulerState: attempt.SchedulerState}
		latest := true
		if selected {
			// A selected older attempt shows its own outcome; the summary
			// result belongs to the latest attempt.
			latestAttemptID, _ := state.LatestAttemptID(runDir, jobSpec.ID)
			latest = selectedAttemptID == latestAttemptID
			job.AttemptID = selectedAttemptID
			job.SubmittedAt = state.ReadAttemptTimestamp(jobDir, "submitted_at")
			job.FinishedAt = state.ReadAttemptTimestamp(jobDir, "finished_at")
		}
		if result, ok := jobstatus.ResolveAttempt(attempt, latest, summaryResult, hasSummary).Result(jobSpec); ok {
			if selected {
				result.AttemptID = selectedAttemptID
			}
			job.Result = &result
		}
		if job.Result != nil {
			job.DiagnosisOutdated = diagnose.Outdated(*job.Result)
		}
		if job.Result != nil && job.FinishedAt == "" && attempt.HasWrapper {
			job.FinishedAt = attempt.Wrapper.FinishedAt
		}
		job.Attempts = loadAttempts(store, runDir, jobSpec)
		jobs = append(jobs, job)
		delete(results, jobSpec.ID)
	}
	for _, result := range summary.Results {
		if _, exists := results[result.ID]; !exists {
			continue
		}
		resultCopy := result
		jobs = append(jobs, Job{ID: result.ID, Command: result.Command, Result: &resultCopy, DiagnosisOutdated: diagnose.Outdated(result), SubmittedAt: state.ReadJobTimestamp(runDir, result.ID, "submitted_at"), FinishedAt: state.ReadJobTimestamp(runDir, result.ID, "finished_at")})
	}
	return jobs, nil
}

func loadAttempts(store state.Store, runDir string, jobSpec model.JobSpec) []Attempt {
	ids := state.ListAttemptIDs(runDir, jobSpec.ID)
	if len(ids) == 0 {
		return nil
	}
	attempts := make([]Attempt, 0, len(ids))
	for index := len(ids) - 1; index >= 0; index-- {
		attemptID := ids[index]
		jobDir, err := state.SpecificAttemptJobDir(runDir, jobSpec.ID, attemptID)
		if err != nil {
			continue
		}
		outcome := jobstatus.ReadAttempt(store, jobDir)
		attempt := Attempt{ID: attemptID, SubmittedAt: state.ReadAttemptTimestamp(jobDir, "submitted_at"), FinishedAt: state.ReadAttemptTimestamp(jobDir, "finished_at"), SchedulerState: outcome.SchedulerState}
		if result, ok := outcome.Result(jobSpec); ok {
			result.AttemptID = attemptID
			attempt.Result = &result
			if attempt.FinishedAt == "" && outcome.HasWrapper {
				attempt.FinishedAt = outcome.Wrapper.FinishedAt
			}
		}
		attempts = append(attempts, attempt)
	}
	return attempts
}

func buildTimeline(summary model.RunSummary, jobs []Job) []TimelinePoint {
	inputs := make([]JobTimelineInput, 0, len(jobs))
	for _, job := range jobs {
		inputs = append(inputs, JobTimelineInput{Finished: job.Result != nil, Carried: job.Origin != nil, SubmittedAt: job.SubmittedAt, FinishedAt: job.FinishedAt, Success: job.Result != nil && job.Result.ExitCode == 0})
	}
	return BuildTimeline(summary.StartedAt, inputs)
}
