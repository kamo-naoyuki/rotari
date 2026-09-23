package web

import (
	"fmt"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type JobLoader struct {
	Origins             map[string]*model.JobOrigin
	LatestAttemptDir    func(jobID string) (string, error)
	SpecificAttemptDir  func(jobID, attemptID string) (string, error)
	ListAttemptIDs      func(jobID string) []string
	ReadTimestamp       func(jobDir, name string) string
	ReadJobTimestamp    func(jobID, name string) string
	LoadSchedulerState  func(jobDir string) string
	LoadSchedulerResult func(jobDir string, job model.JobSpec) (model.JobResult, bool)
	SchedulerFinishedAt func(jobDir string) string
	LoadLocalResult     func(jobDir string, job model.JobSpec) (model.JobResult, bool)
	LoadTerminalState   func(jobDir string) (int, bool)
	ResolveTimestamps   func(jobID string, origin *model.JobOrigin) (string, string)
}

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

func LoadJobs(commands model.Queue, summary model.RunSummary, attemptIDs []string, loader JobLoader) ([]Job, error) {
	results := model.ResultsByID(summary.Results)
	taskJobs := model.QueueToJobs(commands.Commands)
	selectedAttemptID := ""
	selectedJobID := ""
	if len(attemptIDs) > 0 && attemptIDs[0] != "" {
		selectedAttemptID = attemptIDs[0]
		if payload, err := state.DecodeAttemptID(selectedAttemptID); err == nil {
			selectedJobID = payload.JobID
		}
	}
	jobs := make([]Job, 0, len(taskJobs))
	for _, jobSpec := range taskJobs {
		jobDir, pathErr := loader.LatestAttemptDir(jobSpec.ID)
		if jobSpec.ID == selectedJobID {
			jobDir, pathErr = loader.SpecificAttemptDir(jobSpec.ID, selectedAttemptID)
		}
		if pathErr != nil {
			return nil, fmt.Errorf("invalid job ID %q: %w", jobSpec.ID, pathErr)
		}
		origin := loader.Origins[jobSpec.ID]
		submittedAt, finishedAt := loader.ResolveTimestamps(jobSpec.ID, origin)
		attemptID := jobSpec.AttemptID
		if attemptID == "" {
			if result, ok := results[jobSpec.ID]; ok {
				attemptID = result.AttemptID
			}
		}
		job := Job{ID: jobSpec.ID, AttemptID: attemptID, AttemptDir: jobDir, Name: jobSpec.Name, Stage: jobSpec.Stage, Command: jobSpec.Command, WorkingDirectory: jobSpec.WorkingDirectory, Executor: jobSpec.Executor, ExecutorOptions: jobSpec.ExecutorOptions, DependsOn: jobSpec.DependsOn, Origin: origin, ArrayTaskID: jobSpec.ArrayTaskID, ArrayFirst: jobSpec.ArrayFirst, ArrayLast: jobSpec.ArrayLast, SubmittedAt: submittedAt, FinishedAt: finishedAt, SchedulerState: loader.LoadSchedulerState(jobDir)}
		if jobSpec.ID == selectedJobID {
			job.AttemptID = selectedAttemptID
		}
		if result, ok := results[jobSpec.ID]; ok {
			job.Result = &result
		} else if result, ok := loader.LoadSchedulerResult(jobDir, jobSpec); ok {
			job.Result = &result
			if job.FinishedAt == "" {
				job.FinishedAt = loader.SchedulerFinishedAt(jobDir)
			}
		} else if exitCode, ok := loader.LoadTerminalState(jobDir); ok {
			job.Result = &model.JobResult{ID: jobSpec.ID, Command: jobSpec.Command, ExitCode: exitCode}
		} else if result, ok := loader.LoadLocalResult(jobDir, jobSpec); ok {
			job.Result = &result
		}
		if jobSpec.ID == selectedJobID {
			if result, ok := loader.LoadLocalResult(jobDir, jobSpec); ok {
				result.AttemptID = selectedAttemptID
				job.Result = &result
			}
			job.SubmittedAt = loader.ReadTimestamp(jobDir, "submitted_at")
			job.FinishedAt = loader.ReadTimestamp(jobDir, "finished_at")
		}
		job.Attempts = loadAttempts(jobSpec, loader)
		jobs = append(jobs, job)
		delete(results, jobSpec.ID)
	}
	for _, result := range summary.Results {
		if _, exists := results[result.ID]; !exists {
			continue
		}
		resultCopy := result
		jobs = append(jobs, Job{ID: result.ID, Command: result.Command, Result: &resultCopy, SubmittedAt: loader.ReadJobTimestamp(result.ID, "submitted_at"), FinishedAt: loader.ReadJobTimestamp(result.ID, "finished_at")})
	}
	return jobs, nil
}

func loadAttempts(jobSpec model.JobSpec, loader JobLoader) []Attempt {
	ids := loader.ListAttemptIDs(jobSpec.ID)
	if len(ids) == 0 {
		return nil
	}
	attempts := make([]Attempt, 0, len(ids))
	for index := len(ids) - 1; index >= 0; index-- {
		attemptID := ids[index]
		jobDir, err := loader.SpecificAttemptDir(jobSpec.ID, attemptID)
		if err != nil {
			continue
		}
		attempt := Attempt{ID: attemptID, SubmittedAt: loader.ReadTimestamp(jobDir, "submitted_at"), FinishedAt: loader.ReadTimestamp(jobDir, "finished_at"), SchedulerState: loader.LoadSchedulerState(jobDir)}
		if result, ok := loader.LoadLocalResult(jobDir, jobSpec); ok {
			result.AttemptID = attemptID
			attempt.Result = &result
		} else if result, ok := loader.LoadSchedulerResult(jobDir, jobSpec); ok {
			result.AttemptID = attemptID
			attempt.Result = &result
			if attempt.FinishedAt == "" {
				attempt.FinishedAt = loader.SchedulerFinishedAt(jobDir)
			}
		} else if exitCode, ok := loader.LoadTerminalState(jobDir); ok {
			attempt.Result = &model.JobResult{ID: jobSpec.ID, AttemptID: attemptID, Command: jobSpec.Command, ExitCode: exitCode}
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
