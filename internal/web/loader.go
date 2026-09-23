package web

import (
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/model"
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

func buildTimeline(summary model.RunSummary, jobs []Job) []TimelinePoint {
	inputs := make([]JobTimelineInput, 0, len(jobs))
	for _, job := range jobs {
		inputs = append(inputs, JobTimelineInput{Finished: job.Result != nil, Carried: job.Origin != nil, SubmittedAt: job.SubmittedAt, FinishedAt: job.FinishedAt, Success: job.Result != nil && job.Result.ExitCode == 0})
	}
	return BuildTimeline(summary.StartedAt, inputs)
}
