package jobcontrol

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// RetrySelection describes which active-run jobs a manual retry asks to
// reopen. Eligibility (final, executed by this run, and not already active)
// remains an engine decision and is reported in its response.
type RetrySelection struct {
	JobIDs       []string
	Names        []string
	Selection    string
	Scope        model.CommandSelector
	Filter       jobfilter.Filter
	PartialArray bool
}

// SelectRetry resolves an active run's retry selectors to job IDs. It uses
// jobfilter for result and per-job rules and jobstatus for execution facts;
// the engine later decides which selected jobs are currently eligible.
func (controller Controller) SelectRetry(baseDir, project, runID string, selection RetrySelection, now time.Time) (string, []string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		return "", nil, err
	}
	lock, err := activeLock(paths, project, runID)
	if err != nil {
		return "", nil, err
	}
	return controller.selectRetryRun(paths, lock.RunID, selection, now)
}

func (controller Controller) selectRetryRun(paths state.ProjectPaths, runID string, selection RetrySelection, now time.Time) (string, []string, error) {
	snapshot, err := loadRetrySnapshot(controller, paths, runID)
	if err != nil {
		return "", nil, err
	}
	intent, err := normalizeRetryIntent(snapshot, selection)
	if err != nil {
		return "", nil, err
	}
	selected, err := selectRetryTargets(controller, snapshot, intent, selection, now)
	if err != nil {
		return "", nil, err
	}
	if len(selected) == 0 {
		return "", nil, errors.New("no jobs match retry selection in active run")
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return snapshot.runID, ids, nil
}

type retryRunSnapshot struct {
	paths    state.ProjectPaths
	runID    string
	commands []model.QueuedCommand
	jobs     map[string]runviewJob
}

type retryIntent struct {
	requested         map[string]bool
	commandsRequested map[string]bool
	scopeIndexes      map[int]bool
	hasScope          bool
}

func loadRetrySnapshot(controller Controller, paths state.ProjectPaths, runID string) (retryRunSnapshot, error) {
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil || lock.RunID != runID {
		return retryRunSnapshot{}, fmt.Errorf("run %q is no longer active", runID)
	}
	runDir, err := runDirectory(paths, lock)
	if err != nil {
		return retryRunSnapshot{}, err
	}
	loaded, err := runview.LoadRun(paths, lock.RunID, controller.Store)
	if err != nil {
		return retryRunSnapshot{}, err
	}
	jobs := make(map[string]runviewJob, len(loaded.Jobs))
	for _, job := range loaded.Jobs {
		jobs[job.Spec.ID] = runviewJob{spec: job.Spec, result: job.Result, final: job.Final, origin: job.Origin, carried: job.Carried, exists: true}
	}
	queue, err := loadCommandSnapshot(runDir)
	if err != nil {
		return retryRunSnapshot{}, err
	}
	return retryRunSnapshot{paths: paths, runID: lock.RunID, commands: queue.Commands, jobs: jobs}, nil
}

func normalizeRetryIntent(snapshot retryRunSnapshot, selection RetrySelection) (retryIntent, error) {
	intent := retryIntent{
		requested:         make(map[string]bool, len(selection.JobIDs)),
		commandsRequested: make(map[string]bool),
		scopeIndexes:      make(map[int]bool),
		hasScope:          selection.Scope.Kinds() > 0,
	}
	requested := make(map[string]bool, len(selection.JobIDs))
	commandsRequested := make(map[string]bool)
	for _, rawID := range selection.JobIDs {
		id := rawID
		if payload, decodeErr := state.DecodeAttemptID(rawID); decodeErr == nil {
			if payload.RunID != snapshot.runID {
				return retryIntent{}, fmt.Errorf("attempt %q belongs to run %q, not %q", rawID, payload.RunID, snapshot.runID)
			}
			id = payload.JobID
		}
		if findCommand(snapshot.commands, id) {
			commandsRequested[id] = true
			continue
		}
		requested[id] = true
	}
	nameIDs, missingNames := resolve.JobIDsByName(snapshot.commands, selection.Names)
	if len(missingNames) > 0 {
		return retryIntent{}, fmt.Errorf("job name %q not found in active run %q", missingNames[0], snapshot.runID)
	}
	for id := range nameIDs {
		requested[id] = true
	}
	scopeIndexes, err := retryScopeIndexes(snapshot.commands, selection.Scope)
	if err != nil {
		return retryIntent{}, err
	}
	intent.scopeIndexes = scopeIndexes
	intent.requested = requested
	intent.commandsRequested = commandsRequested
	return intent, nil
}

func retryScopeIndexes(commands []model.QueuedCommand, scope model.CommandSelector) (map[int]bool, error) {
	indexes := make(map[int]bool)
	if scope.Kinds() > 1 {
		return nil, errors.New("--stage cannot be combined with --matrix")
	}
	if scope.Kinds() == 0 {
		return indexes, nil
	}
	selected, err := model.SelectCommands(commands, scope)
	if err != nil {
		return nil, err
	}
	for _, index := range selected {
		indexes[index] = true
	}
	return indexes, nil
}

func selectRetryTargets(controller Controller, snapshot retryRunSnapshot, intent retryIntent, selection RetrySelection, now time.Time) (map[string]bool, error) {
	selected := make(map[string]bool)
	for index, command := range snapshot.commands {
		if intent.hasScope && !intent.scopeIndexes[index] || !selection.Filter.MatchesCommand(command) {
			continue
		}
		tasks := model.QueueToJobs([]model.QueuedCommand{command})
		ids, err := selectRetryCommand(controller, snapshot, command, tasks, intent, selection, now)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			selected[id] = true
		}
	}
	for id := range intent.requested {
		if !selected[id] && !snapshot.jobs[id].exists {
			return nil, fmt.Errorf("job ID %q not found in active run %q", id, snapshot.runID)
		}
	}
	return selected, nil
}

func selectRetryCommand(controller Controller, snapshot retryRunSnapshot, command model.QueuedCommand, tasks []model.JobSpec, intent retryIntent, selection RetrySelection, now time.Time) ([]string, error) {
	if command.Array == nil {
		fact := retryFilterJob(controller, snapshot.paths, snapshot.runID, snapshot.jobs[command.ID], now)
		if intent.commandsRequested[command.ID] || intent.requested[command.ID] || selection.Filter.Selects(selection.Selection, fact) {
			return []string{command.ID}, nil
		}
		return nil, nil
	}
	return selectRetryArray(controller, snapshot, command, tasks, intent, selection, now)
}

func selectRetryArray(controller Controller, snapshot retryRunSnapshot, command model.QueuedCommand, tasks []model.JobSpec, intent retryIntent, selection RetrySelection, now time.Time) ([]string, error) {
	matched := make(map[string]bool, len(tasks))
	facts := make([]jobfilter.Job, 0, len(tasks))
	anyTask := false
	for _, task := range tasks {
		fact := retryFilterJob(controller, snapshot.paths, snapshot.runID, snapshot.jobs[task.ID], now)
		facts = append(facts, fact)
		matched[task.ID] = intent.commandsRequested[command.ID] || intent.requested[task.ID] || selection.Filter.Selects(selection.Selection, fact)
		anyTask = anyTask || matched[task.ID]
	}
	whole := !selection.PartialArray && (intent.commandsRequested[command.ID] || anyTask || selection.Filter.SelectsArray(selection.Selection, facts))
	if whole {
		for _, task := range tasks {
			if snapshot.jobs[task.ID].carried {
				return nil, fmt.Errorf("job %q is carried from an earlier run and is not executed by active run %q", task.ID, snapshot.runID)
			}
		}
	}
	selected := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if whole || matched[task.ID] {
			selected = append(selected, task.ID)
		}
	}
	return selected, nil
}

type runviewJob struct {
	spec    model.JobSpec
	result  model.JobResult
	final   bool
	origin  *model.JobOrigin
	carried bool
	exists  bool
}

func retryFilterJob(controller Controller, paths state.ProjectPaths, runID string, job runviewJob, now time.Time) jobfilter.Job {
	origin := model.JobOrigin{RunID: runID, JobID: job.spec.ID}
	if job.origin != nil {
		origin = *job.origin
	}
	fact := jobstatus.FilterJob(controller.Store, paths.RunsDir, origin, job.spec.ID, job.result, job.final, now)
	return fact
}

func findCommand(commands []model.QueuedCommand, id string) bool {
	for _, command := range commands {
		if command.ID == id {
			return true
		}
	}
	return false
}
