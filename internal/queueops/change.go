package queueops

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Mutation is what change sets on each selected job. Empty values leave a
// field unchanged; a Clear field empties it.
type Mutation struct {
	Executor              string
	ExecutorOptions       []string
	ClearExecutorOptions  bool
	Environment           []string
	ClearEnvironment      bool
	WorkingDirectory      string
	ClearWorkingDirectory bool
	SetJobName            string
	DependsOn             []string
	ClearDependsOn        bool
	// DependsOnFinished replaces DependsOnFinished when non-empty.
	DependsOnFinished      []string
	ClearDependsOnFinished bool
	// Artifacts replaces the declared artifact paths when non-empty.
	Artifacts      []string
	ClearArtifacts bool
	// Timeout replaces Timeout when non-empty.
	Timeout      string
	ClearTimeout bool
	// Retry replaces Retry when set; ClearRetry also clears the delay
	// settings.
	Retry      *int
	ClearRetry bool
	// RetryDelay, RetryBackoff, and RetryMaxDelay replace their fields when
	// set.
	RetryDelay    string
	RetryBackoff  float64
	RetryMaxDelay string
	// Status marks the job with a status in place of its recorded result's
	// when set; ClearStatus removes the mark. See model.MarkResult.
	Status      string
	ClearStatus bool
	Command     []string
}

// Change applies mutation to the selected jobs of the current queue, or of
// the batch restored from requestedRunID. It returns one line per changed
// job.
func (editor Editor) Change(baseDir, projectName, requestedRunID string, selector model.CommandSelector, mutation Mutation) (string, error) {
	return editor.ChangeWithFilter(baseDir, projectName, requestedRunID, selector, jobfilter.Filter{}, mutation)
}

// ChangeWithFilter applies mutation to the selected jobs, narrowing the
// selector with a filter before writing the queue.
func (editor Editor) ChangeWithFilter(baseDir, projectName, requestedRunID string, selector model.CommandSelector, filter jobfilter.Filter, mutation Mutation) (string, error) {
	if mutation.Executor != "" && !editor.Executors.Known(mutation.Executor) {
		return "", fmt.Errorf("unsupported executor: %s", mutation.Executor)
	}
	if err := model.ValidateEnvironment(mutation.Environment); err != nil {
		return "", fmt.Errorf("invalid environment: %w", err)
	}
	if mutation.Status != "" && !model.ValidMarkedStatus(mutation.Status) {
		return "", fmt.Errorf("invalid status %q (choose success, failed, cancelled, or unfinished)", mutation.Status)
	}
	if mutation.Status != "" && mutation.ClearStatus {
		return "", errors.New("a status and clearing the status cannot be combined")
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", err
	}
	var changedIDs, lines []string
	var restored string
	err = project.EditQueueGuarded(paths, editor.Guard, func(queue *model.Queue) error {
		var err error
		if restored, err = restoreSnapshot(paths, requestedRunID, queue); err != nil {
			return err
		}
		if err := checkEditable(*queue, projectName, selector); err != nil {
			return err
		}
		indexes, err := model.SelectCommands(queue.Commands, selector)
		if err != nil {
			return err
		}
		if !filter.Empty() {
			filtered := make([]int, 0, len(indexes))
			for _, jobIndex := range indexes {
				if filter.MatchesCommand(queue.Commands[jobIndex]) {
					filtered = append(filtered, jobIndex)
				}
			}
			indexes = filtered
		}
		var groupIDs []string
		before := make(map[int]model.JobSpec, len(indexes))
		for _, jobIndex := range indexes {
			before[jobIndex] = commandSpec(queue.Commands[jobIndex])
			if matrix := queue.Commands[jobIndex].Matrix; matrix != nil {
				groupIDs = append(groupIDs, matrix.GroupID)
			}
			if err := applyMutation(*queue, jobIndex, mutation); err != nil {
				return err
			}
		}
		changed := make([]model.QueuedCommand, 0, len(indexes))
		for _, jobIndex := range indexes {
			changed = append(changed, queue.Commands[jobIndex])
		}
		if err := model.ValidateReservedNames(changed); err != nil {
			return err
		}
		// A group changed the same way throughout still matches its
		// provenance; a partly changed one no longer does.
		model.ClearInconsistentMatrixGroups(queue.Commands, groupIDs)
		if err := ValidateJobs(*queue); err != nil {
			return err
		}
		if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
			return fmt.Errorf("invalid dependencies: %w", err)
		}
		changedIDs = changedIDs[:0]
		lines = lines[:0]
		for _, jobIndex := range indexes {
			changedIDs = append(changedIDs, queue.Commands[jobIndex].ID)
			lines = append(lines, fmt.Sprintf("changed queue=%s job=%s%s", projectName, queue.Commands[jobIndex].ID,
				formatChanges(runlineage.SpecChanges(before[jobIndex], commandSpec(queue.Commands[jobIndex])))))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if restored != "" {
		lines = append([]string{restored}, lines...)
	}
	if editor.Warn != nil && len(mutation.Command) > 0 {
		for _, warning := range unexpandedVariableWarning([]model.QueuedCommand{{Command: mutation.Command}}) {
			editor.Warn(warning)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// commandSpec returns the definition of command that a run compares: an
// array's first task stands for the array, whose tasks share it.
func commandSpec(command model.QueuedCommand) model.JobSpec {
	if jobs := model.QueueToJobs([]model.QueuedCommand{command}); len(jobs) > 0 {
		return jobs[0]
	}
	return model.JobSpec{}
}

// formatChanges renders definition changes as key=value fields, as
// " timeout=5s->60s environment+=LR=0.1", so a preview shows what a change
// does; a status mark, which is not a definition, is not among them.
func formatChanges(changes []runlineage.Change) string {
	var builder strings.Builder
	for _, change := range changes {
		if len(change.Added) > 0 || len(change.Removed) > 0 {
			if len(change.Added) > 0 {
				fmt.Fprintf(&builder, " %s+=%s", change.Field, strings.Join(change.Added, ","))
			}
			if len(change.Removed) > 0 {
				fmt.Fprintf(&builder, " %s-=%s", change.Field, strings.Join(change.Removed, ","))
			}
			continue
		}
		from, to := change.From, change.To
		if from == "" {
			from = "-"
		}
		if to == "" {
			to = "-"
		}
		fmt.Fprintf(&builder, " %s=%s->%s", change.Field, from, to)
	}
	return builder.String()
}

func applyMutation(queue model.Queue, jobIndex int, mutation Mutation) error {
	changed := &queue.Commands[jobIndex]
	if mutation.Status != "" || mutation.ClearStatus {
		// A mark covers the whole command, replacing any task's own.
		changed.MarkedStatus = mutation.Status
		changed.TaskMarkedStatus = nil
	}
	if mutation.Executor != "" {
		changed.Executor = mutation.Executor
	}
	if len(mutation.ExecutorOptions) > 0 || mutation.ClearExecutorOptions {
		changed.ExecutorOptions = append([]string(nil), mutation.ExecutorOptions...)
	}
	if len(mutation.Environment) > 0 || mutation.ClearEnvironment {
		changed.Environment = append([]string(nil), mutation.Environment...)
	}
	if mutation.WorkingDirectory != "" || mutation.ClearWorkingDirectory {
		changed.WorkingDirectory = mutation.WorkingDirectory
	}
	if mutation.SetJobName != "" && mutation.SetJobName != changed.Name {
		if err := validateRename(queue, jobIndex, mutation.SetJobName); err != nil {
			return err
		}
		changed.Name = mutation.SetJobName
	}
	if len(mutation.Command) > 0 {
		changed.Command = append([]string(nil), mutation.Command...)
	}
	if len(mutation.DependsOn) > 0 || mutation.ClearDependsOn {
		changed.DependsOn = append([]string(nil), mutation.DependsOn...)
	}
	if len(mutation.DependsOnFinished) > 0 || mutation.ClearDependsOnFinished {
		changed.DependsOnFinished = append([]string(nil), mutation.DependsOnFinished...)
	}
	if len(mutation.Artifacts) > 0 || mutation.ClearArtifacts {
		changed.Artifacts = append([]string(nil), mutation.Artifacts...)
	}
	if mutation.Timeout != "" || mutation.ClearTimeout {
		changed.Timeout = mutation.Timeout
	}
	if mutation.Retry != nil || mutation.ClearRetry {
		changed.Retry = mutation.Retry
	}
	if mutation.ClearRetry {
		changed.RetryDelay, changed.RetryBackoff, changed.RetryMaxDelay = "", 0, ""
	}
	if mutation.RetryDelay != "" {
		changed.RetryDelay = mutation.RetryDelay
	}
	if mutation.RetryBackoff != 0 {
		changed.RetryBackoff = mutation.RetryBackoff
	}
	if mutation.RetryMaxDelay != "" {
		changed.RetryMaxDelay = mutation.RetryMaxDelay
	}
	return nil
}

func validateRename(queue model.Queue, jobIndex int, newName string) error {
	for index, command := range queue.Commands {
		if index == jobIndex {
			continue
		}
		for _, job := range model.QueueToJobs([]model.QueuedCommand{command}) {
			if job.Name == newName {
				return fmt.Errorf("job name %q is already in use", newName)
			}
		}
	}
	oldName := queue.Commands[jobIndex].Name
	for index, command := range queue.Commands {
		if index == jobIndex {
			continue
		}
		for _, dependency := range command.AllDependencies() {
			if dependency == oldName {
				return fmt.Errorf("job %q is referenced by dependency; rename is not allowed", oldName)
			}
		}
	}
	return nil
}

// restoreSnapshot replaces queue with the command snapshot of
// requestedRunID, or leaves it alone when requestedRunID is empty. It returns
// a line that says so, for the edit's output, or "" when it leaves the queue.
func restoreSnapshot(paths state.ProjectPaths, requestedRunID string, queue *model.Queue) (string, error) {
	if requestedRunID == "" {
		return "", nil
	}
	runID, err := resolve.RunID(paths, requestedRunID)
	if err != nil {
		return "", err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", err
	}
	snapshot, err := state.ReadQueueFile(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return "", fmt.Errorf("failed to load command snapshot: %w", err)
	}
	if len(snapshot.Commands) == 0 {
		return "", errors.New("command snapshot has no jobs")
	}
	// Marks applied to that run, whose results already show them.
	for index := range snapshot.Commands {
		snapshot.Commands[index].MarkedStatus = ""
		snapshot.Commands[index].TaskMarkedStatus = nil
	}
	message := fmt.Sprintf("restored queue=%s from run=%s jobs=%d, replacing %d queued job(s)", paths.ProjectName, runID, len(model.QueueToJobs(snapshot.Commands)), len(model.QueueToJobs(queue.Commands)))
	*queue = snapshot
	return message, nil
}

// checkEditable rejects a change or remove on an empty queue, which never
// restores a run on its own (copy or --run-id restores one explicitly), and
// an attempt ID in selector, which names an attempt rather than a queued job.
func checkEditable(queue model.Queue, projectName string, selector model.CommandSelector) error {
	if len(queue.Commands) == 0 {
		return fmt.Errorf("project %q has no queued jobs; restore a run with 'rotari copy' or pass --run-id", projectName)
	}
	for _, id := range selector.IDs {
		if payload, err := state.DecodeAttemptID(id); err == nil {
			return fmt.Errorf("%s is an attempt ID; queue edits take a job ID, such as %s", id, payload.JobID)
		}
	}
	return nil
}
