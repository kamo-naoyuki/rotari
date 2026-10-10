package web

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type QueueLoader struct {
	ProjectName string
	Paths       state.ProjectPaths
	Queue       func() (model.Queue, error)
	Lock        func() (model.LockInfo, error)
	Runs        func() ([]string, error)
	Summary     func(runID string) (model.RunSummary, error)
	Jobs        func(runID string, summary model.RunSummary) ([]Job, error)
	Context     func(runID string) (model.RunContext, error)
	Sources     func(runID string) (model.RunSources, bool, error)
	Notes       func(runID string) ([]model.RunNote, error)
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
	state.RunCount = len(runIDs)
	for _, runID := range runIDs {
		summary, summaryErr := loader.Summary(runID)
		if newerStateVersion(summaryErr) {
			state.Runs = append(state.Runs, unreadableRun(runID, summaryErr))
			continue
		}
		if summaryErr != nil {
			summary = model.RunSummary{RunID: runID, Status: "running", StartedAt: runningStartedAt}
		}
		if summary.RunID == "" {
			summary.RunID = runID
		}
		jobs, err := loader.Jobs(runID, summary)
		if newerStateVersion(err) {
			state.Runs = append(state.Runs, unreadableRun(runID, err))
			continue
		}
		if err != nil {
			return QueueState{}, err
		}
		context, contextErr := loader.Context(runID)
		if contextErr != nil {
			context = model.RunContext{}
		}
		context.LoadSamples = loader.Samples(runID)
		var notes []model.RunNote
		if loader.Notes != nil {
			if loaded, notesErr := loader.Notes(runID); notesErr == nil {
				notes = loaded
			}
		}
		for index := range jobs {
			jobs[index].NoteLabels = jobNoteLabels(notes, jobs[index])
		}
		var sources []model.SourceRevision
		if loader.Sources != nil {
			if recorded, ok, sourcesErr := loader.Sources(runID); sourcesErr == nil && ok {
				sources = recorded.Sources
			}
		}
		lifecycle := summary.Status
		clientStatus := model.RunClientStatus{State: "unknown"}
		if loader.Paths.ProjectDir != "" {
			if resolved, statusErr := runview.RunLifecycleLabel(loader.Paths, runID); statusErr == nil {
				lifecycle = resolved
			}
			if resolved, statusErr := runview.ClientStatus(loader.Paths, runID); statusErr == nil {
				clientStatus = resolved
			}
		}
		state.Runs = append(state.Runs, Run{
			RunSummary: summary, LineageSummary: buildLineageSummary(summary, jobs), Jobs: jobs,
			Lifecycle: lifecycle, ClientStatus: clientStatus,
			ClientLabel: runview.ClientStatusLabel(clientStatus),
			CWD:         context.CWD, Context: context, Timeline: buildTimeline(summary, jobs, context.LoadSamples), Running: runID == state.RunningRunID,
			Sources: sources, SourceLabels: sourceLabels(sources), Notes: notes, NoteLabels: runNoteLabels(notes),
		})
	}
	sort.Slice(state.Runs, func(i, j int) bool { return state.Runs[i].RunID > state.Runs[j].RunID })
	return state, nil
}

func buildLineageSummary(summary model.RunSummary, jobs []Job) runlineage.RunSummary {
	run := runlineage.Run{ID: summary.RunID, Name: summary.RunName, StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt}
	for _, job := range jobs {
		status := job.lineageStatus
		diagnoses := make([]string, 0)
		diagnosisStatus := ""
		result := model.JobResult{}
		if job.Result != nil {
			result = *job.Result
			diagnosisStatus = job.Result.DiagnosisStatus
			for _, diagnosis := range job.Result.Diagnoses {
				diagnoses = append(diagnoses, diagnosis.Name)
			}
		}
		run.Jobs = append(run.Jobs, runlineage.Job{
			Spec: model.JobSpec{ID: job.ID, Name: job.Name, ArrayTaskID: job.ArrayTaskID, Command: job.Command, WorkingDirectory: job.WorkingDirectory,
				Executor: job.Executor, ExecutorOptions: job.ExecutorOptions, Stage: job.Stage, DependsOn: job.DependsOn, DependsOnFinished: job.DependsOnFinished},
			Status: status, Origin: job.Origin, DiagnosisStatus: diagnosisStatus, Diagnoses: diagnoses, Result: result, Carried: job.Carried,
		})
	}
	return runlineage.RunSummary{Run: runlineage.RunInfo{ID: summary.RunID, Name: summary.RunName}, Counts: runlineage.Summarize(run), Diagnoses: runlineage.SummarizeDiagnoses(run), Failures: runlineage.FailureGroups(run), Origins: runlineage.SummarizeOrigins(run)}
}

// newerStateVersion reports whether err is a run file from a newer rotari.
func newerStateVersion(err error) bool {
	return errors.Is(err, state.ErrNewerStateVersion)
}

// unreadableRun keeps a run whose files come from a newer rotari in the list,
// with the reason, so the project's other runs still show.
func unreadableRun(runID string, err error) Run {
	return Run{RunSummary: model.RunSummary{RunID: runID, Status: "unreadable"}, Jobs: []Job{}, Unreadable: err.Error()}
}

// LoadRunJobs reads runDir's command snapshot and projects its jobs with
// LoadJobs.
func LoadRunJobs(store state.Store, runDir string, summary model.RunSummary, selectedAttemptID string) ([]Job, error) {
	commands, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil, err
	}
	return LoadJobs(store, runDir, commands, summary, selectedAttemptID)
}

// LoadJobs projects a run's jobs for the Web UI. Each job's result follows
// the shared jobstatus fallback chain. A non-empty selectedAttemptID shows
// that attempt, instead of the latest one, for its job.
func LoadJobs(store state.Store, runDir string, commands model.Queue, summary model.RunSummary, selectedAttemptID string) ([]Job, error) {
	// Before the run writes its summary, callers pass one without results,
	// and the results the run carries are its recorded ones.
	var recorded *model.RunSummary
	if len(summary.Results) > 0 {
		recorded = &summary
	}
	results := jobstatus.RecordedResults(runDir, recorded)
	origins := model.QueueOriginsByJobID(commands)
	taskJobs := model.QueueToJobs(commands.Commands)
	selectedJobID := ""
	if selectedAttemptID != "" {
		if payload, err := state.DecodeAttemptID(selectedAttemptID); err == nil {
			selectedJobID = payload.JobID
		}
	}
	matrices := make(map[string]*Matrix)
	for _, command := range commands.Commands {
		if spec := command.Matrix; spec != nil {
			matrices[command.ID] = &Matrix{GroupID: spec.GroupID, BaseName: spec.BaseName, Dimensions: spec.Dimensions, Values: spec.Values}
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
		latestAttemptID, _ := state.LatestAttemptID(runDir, jobSpec.ID)
		origin := origins[jobSpec.ID]
		summaryResult, hasSummary := results[jobSpec.ID]
		attemptID := jobSpec.AttemptID
		if attemptID == "" && hasSummary {
			attemptID = summaryResult.AttemptID
		}
		attempt := jobstatus.ReadAttempt(store, jobDir)
		_, finalErr := os.Stat(filepath.Join(jobDir, state.FinalResultFileName))
		job := Job{ID: jobSpec.ID, AttemptID: attemptID, AttemptDir: jobDir, Name: jobSpec.Name, Stage: jobSpec.Stage, Command: jobSpec.Command, Environment: jobSpec.Environment, WorkingDirectory: jobSpec.WorkingDirectory, Executor: jobSpec.Executor, ExecutorOptions: jobSpec.ExecutorOptions, LogMode: jobSpec.LogMode, DependsOn: jobSpec.DependsOn, DependsOnFinished: jobSpec.DependsOnFinished, Origin: origin, ArrayTaskID: jobSpec.ArrayTaskID, ArrayFirst: jobSpec.ArrayFirst, ArrayLast: jobSpec.ArrayLast, SchedulerState: attempt.SchedulerState, Final: hasSummary || finalErr == nil}
		job.Matrix = matrices[jobSpec.ID]
		if jobSpec.ArrayGroup != "" {
			job.Matrix = matrices[jobSpec.ArrayGroup]
		}
		latest := true
		if selected {
			// A selected older attempt shows its own outcome; the summary
			// result belongs to the latest attempt.
			latest = selectedAttemptID == latestAttemptID
			job.AttemptID = selectedAttemptID
		}
		resolved := jobstatus.ResolveAttempt(attempt, latest, summaryResult, hasSummary)
		job.Carried = runlineage.IsCarried(origin, latestAttemptID, resolved.Blocked(), hasSummary)
		if selected {
			job.SubmittedAt = state.ResolveAttemptTimestamp(jobDir, "submitted_at")
			job.FinishedAt = state.ResolveAttemptTimestamp(jobDir, "finished_at")
		} else {
			job.SubmittedAt, job.FinishedAt = jobstatus.Timestamps(runDir, jobSpec.ID, origin, job.Carried)
		}
		if result, ok := resolved.Result(jobSpec); ok {
			if selected {
				result.AttemptID = selectedAttemptID
			}
			job.Result = &result
		}
		job.ExecutionStatus = jobstatus.DisplayLabel(resolved.DisplayStatus(jobSpec), resolved.Accepted(), job.Carried)
		job.lineageStatus = runview.LineageStatus(resolved)
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
		// A result without a job in the command snapshot resolves from the
		// summary alone.
		resolved := jobstatus.ResolveJob(jobstatus.Attempt{}, resultCopy, true)
		displayStatus := jobstatus.DisplayLabel(resolved.DisplayStatus(model.JobSpec{ID: result.ID, Command: result.Command}), resolved.Accepted(), false)
		jobs = append(jobs, Job{ID: result.ID, Command: result.Command, Result: &resultCopy, ExecutionStatus: displayStatus, lineageStatus: runview.LineageStatus(resolved), DiagnosisOutdated: diagnose.Outdated(result), SubmittedAt: state.ReadJobTimestamp(runDir, result.ID, "submitted_at"), FinishedAt: state.ReadJobTimestamp(runDir, result.ID, "finished_at")})
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
		attempt := Attempt{ID: attemptID, ExecutionStatus: jobstatus.ResolveJob(outcome, model.JobResult{}, false).DisplayStatus(jobSpec), SubmittedAt: state.ReadAttemptTimestamp(jobDir, "submitted_at"), FinishedAt: state.ReadAttemptTimestamp(jobDir, "finished_at"), SchedulerState: outcome.SchedulerState}
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

func buildTimeline(summary model.RunSummary, jobs []Job, loadSamples []model.LoadSample) []TimelinePoint {
	inputs := make([]JobTimelineInput, 0, len(jobs))
	for _, job := range jobs {
		inputs = append(inputs, JobTimelineInput{Finished: job.Result != nil, Carried: job.Carried, SubmittedAt: job.SubmittedAt, FinishedAt: job.FinishedAt, Success: job.Result != nil && job.Result.ExitCode == 0})
	}
	startedAt := summary.StartedAt
	if len(loadSamples) > 0 {
		sampleAt := loadSamples[0].At
		summaryStart, summaryErr := time.Parse(time.RFC3339, summary.StartedAt)
		sampleStart, sampleErr := time.Parse(time.RFC3339Nano, sampleAt)
		if summaryErr == nil && sampleErr == nil && summaryStart.Sub(sampleStart) > time.Second {
			startedAt = sampleStart.UTC().Format(time.RFC3339)
		}
	}
	return BuildTimeline(startedAt, summary.FinishedAt, inputs)
}

func sourceLabels(sources []model.SourceRevision) []string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		labels = append(labels, model.SourceLabel(source))
	}
	return labels
}

// runNoteLabels describes the notes on the run itself. Notes on a job are
// left to that job's NoteLabels, behind its Notes button.
func runNoteLabels(notes []model.RunNote) []string {
	var labels []string
	for _, note := range model.RunNotesFor(notes, "") {
		labels = append(labels, model.FormatRunNote(note, ""))
	}
	return labels
}

func jobNoteLabels(notes []model.RunNote, job Job) []string {
	var labels []string
	for _, note := range model.RunNotesFor(notes, job.ID) {
		attemptID := note.AttemptID
		note.JobID = ""
		label := model.FormatRunNote(note, "")
		if attemptID != "" && attemptID != job.AttemptID {
			label += " (attempt " + attemptID + ")"
		}
		labels = append(labels, label)
	}
	return labels
}
