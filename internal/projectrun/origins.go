package projectrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ErrNoPreviousRun reports that a selection needs the project's last run but
// the project has none.
var ErrNoPreviousRun = run.ErrNoReferenceRun

// PlanSelection decides which of the queue's jobs a run executes, reading
// earlier results from the project's runs; see run.PlanRerun. referenceRunID
// comes from ReferenceRun.
func (runner Runner) PlanSelection(paths state.ProjectPaths, queue model.Queue, selection string, jobIDs []string, scope model.CommandSelector, filter jobfilter.Filter, referenceRunID string, partialArray bool) (run.Plan, error) {
	plan, err := run.PlanRerun(queue, selection, jobIDs, scope, filter, referenceRunID, partialArray, originResults{paths: paths, store: runner.Store})
	if errors.Is(err, ErrNoPreviousRun) {
		return run.Plan{}, fmt.Errorf("project '%s' has no previous run: %w", paths.ProjectName, err)
	}
	return plan, err
}

// ReferenceRun returns the run whose results a selection falls back to for
// jobs without an origin: requested when given, otherwise the project's last
// run. It is empty without a selection or a previous run. Call it before
// Begin, which makes the new run the last one.
func ReferenceRun(paths state.ProjectPaths, selection, requested string) (string, error) {
	if requested != "" || selection == "" {
		return requested, nil
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	return meta.LastRunID, nil
}

// FingerprintReferenceRun selects the latest settled run for fingerprint
// matching when the caller did not provide one. The metadata fallback matches
// the existing latest-run resolution used by run planning.
func FingerprintReferenceRun(paths state.ProjectPaths, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	return meta.LastRunID, nil
}

// MatchFingerprintQueue enriches queue commands with origins found in a
// historical queue. Fingerprints are calculated from both queues each time;
// no fingerprint is persisted.
func (runner Runner) MatchFingerprintQueue(paths state.ProjectPaths, queue model.Queue, mode, sourceRunID string) (model.Queue, error) {
	switch mode {
	case model.MatchByJobID, model.MatchByFingerprint, model.MatchByIDAndFingerprint:
	default:
		return model.Queue{}, fmt.Errorf("invalid match mode %q", mode)
	}
	if sourceRunID == "" {
		return queue, nil
	}
	runDir, err := state.SafeJoin(paths.RunsDir, sourceRunID)
	if err != nil {
		return model.Queue{}, err
	}
	sourceQueue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return model.Queue{}, fmt.Errorf("failed to load fingerprint source run %q: %w", sourceRunID, err)
	}
	currentJobs, err := model.QueueFingerprintJobs(queue)
	if err != nil {
		return model.Queue{}, err
	}
	sourceJobs, err := model.QueueFingerprintJobs(sourceQueue)
	if err != nil {
		return model.Queue{}, err
	}
	currentJobs, sourceJobs = excludeExistingOrigins(queue, currentJobs, sourceJobs)
	matches := model.MatchFingerprintJobsByMode(currentJobs, sourceJobs, mode)
	results, err := (originResults{paths: paths, store: runner.Store}).RunResults(sourceRunID)
	if errors.Is(err, os.ErrNotExist) {
		results = map[string]model.JobResult{}
	} else if err != nil {
		return model.Queue{}, err
	}
	currentCommands := queueCommandLocations(queue)
	for _, match := range matches {
		location, ok := currentCommands[match.CurrentID]
		if !ok || location.origin(queue.Commands) != nil {
			continue
		}
		origin := fingerprintOrigin(paths, sourceRunID, match.SourceID, results[match.SourceID], results[match.SourceID].AttemptID != "", runner.Store)
		location.setOrigin(queue.Commands, origin)
	}
	return queue, nil
}

func excludeExistingOrigins(queue model.Queue, current, source []model.FingerprintJob) ([]model.FingerprintJob, []model.FingerprintJob) {
	locations := queueCommandLocations(queue)
	protectedCurrent := make(map[string]bool)
	protectedSource := make(map[string]bool)
	for _, job := range current {
		location, ok := locations[job.ID]
		if !ok {
			continue
		}
		origin := location.origin(queue.Commands)
		if origin == nil {
			continue
		}
		protectedCurrent[job.ID] = true
		if origin.JobID != "" {
			protectedSource[origin.JobID] = true
		}
	}
	filter := func(jobs []model.FingerprintJob, protected map[string]bool) []model.FingerprintJob {
		filtered := make([]model.FingerprintJob, 0, len(jobs))
		for _, job := range jobs {
			if !protected[job.ID] {
				filtered = append(filtered, job)
			}
		}
		return filtered
	}
	return filter(current, protectedCurrent), filter(source, protectedSource)
}

type queueCommandLocation struct {
	command int
	task    string
}

func queueCommandLocations(queue model.Queue) map[string]queueCommandLocation {
	locations := make(map[string]queueCommandLocation)
	for index, command := range queue.Commands {
		if command.Array == nil {
			locations[command.ID] = queueCommandLocation{command: index}
			continue
		}
		for _, task := range model.ArrayTaskIDs(command.Array) {
			id := fmt.Sprintf("%s-%d", command.ID, task)
			locations[id] = queueCommandLocation{command: index, task: id}
		}
	}
	return locations
}

func (location queueCommandLocation) origin(commands []model.QueuedCommand) *model.JobOrigin {
	if location.task != "" {
		return commands[location.command].TaskOrigins[location.task]
	}
	return commands[location.command].Origin
}

func (location queueCommandLocation) setOrigin(commands []model.QueuedCommand, origin *model.JobOrigin) {
	if location.task != "" {
		if commands[location.command].TaskOrigins == nil {
			commands[location.command].TaskOrigins = make(map[string]*model.JobOrigin)
		}
		commands[location.command].TaskOrigins[location.task] = origin
		return
	}
	commands[location.command].Origin = origin
}

func fingerprintOrigin(paths state.ProjectPaths, runID, jobID string, result model.JobResult, finished bool, store state.Store) *model.JobOrigin {
	status := model.StatusUnfinished
	if finished {
		status = model.ResultStatus(result, true)
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	return &model.JobOrigin{
		RunID: runID, JobID: jobID, AttemptID: result.AttemptID, Status: status,
		CWD: loadRunCWD(store, runDir), SubmittedAt: state.ReadJobTimestamp(runDir, jobID, "submitted_at"),
		FinishedAt: state.ReadJobTimestamp(runDir, jobID, "finished_at"),
	}
}

func loadRunCWD(store state.Store, runDir string) string {
	context, err := state.LoadContext(store, runDir)
	if err != nil {
		return ""
	}
	return context.CWD
}

// originResults reads origin results from a project's runs.
type originResults struct {
	paths state.ProjectPaths
	store state.Store
}

func (source originResults) RunResults(runID string) (map[string]model.JobResult, error) {
	runDir, err := state.SafeJoin(source.paths.RunsDir, runID)
	if err != nil {
		return nil, err
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return nil, err
	}
	return model.ResultsByID(summary.Results), nil
}

func (source originResults) AttemptResult(origin model.JobOrigin) (model.JobResult, bool, error) {
	return LoadOriginAttemptResult(source.store, source.paths, origin)
}

func (source originResults) Attributes(origin model.JobOrigin) (jobfilter.Attributes, error) {
	runDir, err := state.SafeJoin(source.paths.RunsDir, origin.RunID)
	if err != nil {
		return jobfilter.Attributes{}, err
	}
	jobDir, err := state.LatestAttemptJobDir(runDir, origin.JobID)
	if origin.AttemptID != "" {
		jobDir, err = state.SpecificAttemptJobDir(runDir, origin.JobID, origin.AttemptID)
	}
	if err != nil {
		return jobfilter.Attributes{}, err
	}
	status, _ := executor.LoadWrapperStatus(source.store, filepath.Join(jobDir, "status.json"))
	startedAt, _ := jobfilter.ParseTimestamp(status.StartedAt)
	if startedAt.IsZero() {
		startedAt, _ = jobfilter.ParseTimestamp(state.ReadJobTimestamp(runDir, origin.JobID, "submitted_at"))
	}
	finishedAt, _ := jobfilter.ParseTimestamp(state.ReadJobTimestamp(runDir, origin.JobID, "finished_at"))
	return jobfilter.Attributes{Hosts: status.Hosts, StartedAt: startedAt, FinishedAt: finishedAt, Now: time.Now()}, nil
}

func (source originResults) Log(origin model.JobOrigin) (string, error) {
	runDir, err := state.SafeJoin(source.paths.RunsDir, origin.RunID)
	if err != nil {
		return "", err
	}
	jobDir, err := state.LatestAttemptJobDir(runDir, origin.JobID)
	if origin.AttemptID != "" {
		jobDir, err = state.SpecificAttemptJobDir(runDir, origin.JobID, origin.AttemptID)
	}
	if err != nil {
		return "", err
	}
	for _, name := range []string{"output", state.StdoutFileName, state.StderrFileName} {
		path := filepath.Join(jobDir, name)
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			return string(data), nil
		}
	}
	return "", os.ErrNotExist
}

func (source originResults) Origin(runID, jobID string, result model.JobResult) *model.JobOrigin {
	runDir := filepath.Join(source.paths.RunsDir, runID)
	cwd := ""
	if context, err := state.LoadContext(source.store, runDir); err == nil {
		cwd = context.CWD
	}
	status := "failed"
	if result.ExitCode == 0 {
		status = "success"
	}
	return &model.JobOrigin{
		RunID: runID, JobID: jobID, AttemptID: result.AttemptID, Status: status, CWD: cwd,
		SubmittedAt: state.ReadJobTimestamp(runDir, jobID, "submitted_at"),
		FinishedAt:  state.ReadJobTimestamp(runDir, jobID, "finished_at"),
	}
}

// LoadOriginAttemptResult reads the result of the exact attempt an origin
// names. It reports false when the attempt has not finished.
func LoadOriginAttemptResult(store state.Store, paths state.ProjectPaths, origin model.JobOrigin) (model.JobResult, bool, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, origin.RunID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, origin.JobID, origin.AttemptID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	var job model.JobSpec
	if err := store.ReadJSON(filepath.Join(attemptDir, "command.json"), &job); err != nil {
		return model.JobResult{}, false, fmt.Errorf("failed to load origin attempt %q command: %w", origin.AttemptID, err)
	}
	if result, ok := state.LoadLocalJobResult(attemptDir, job); ok {
		result.AttemptID = origin.AttemptID
		return result, true, nil
	}
	if status, ok := executor.LoadWrapperStatus(store, filepath.Join(attemptDir, "status.json")); ok {
		result := model.JobResult{ID: origin.JobID, AttemptID: origin.AttemptID, Command: job.Command, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}
		return result, true, nil
	}
	return model.JobResult{}, false, nil
}
