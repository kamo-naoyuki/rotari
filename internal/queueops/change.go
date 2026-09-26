package queueops

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
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
	Command       []string
}

// Change applies mutation to the selected jobs of the current queue, or of
// the batch restored from requestedRunID. It returns one line per changed
// job.
func (Editor) Change(baseDir, projectName, requestedRunID string, selector model.CommandSelector, mutation Mutation) (string, error) {
	if err := model.ValidateEnvironment(mutation.Environment); err != nil {
		return "", fmt.Errorf("invalid environment: %w", err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", err
	}
	var changedIDs []string
	err = project.EditQueue(paths, "change", func(queue *model.Queue) error {
		if err := restoreSnapshot(paths, requestedRunID, queue); err != nil {
			return err
		}
		if err := checkEditable(*queue, projectName, selector); err != nil {
			return err
		}
		indexes, err := model.SelectCommands(queue.Commands, selector)
		if err != nil {
			return err
		}
		var groupIDs []string
		for _, jobIndex := range indexes {
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
		for _, jobIndex := range indexes {
			changedIDs = append(changedIDs, queue.Commands[jobIndex].ID)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	lines := make([]string, len(changedIDs))
	for index, id := range changedIDs {
		lines[index] = fmt.Sprintf("changed queue=%s job=%s", projectName, id)
	}
	return strings.Join(lines, "\n"), nil
}

func applyMutation(queue model.Queue, jobIndex int, mutation Mutation) error {
	changed := &queue.Commands[jobIndex]
	before := *changed
	if queue.WorkflowImport {
		changed.Force = true
		changed.Accepted = false
		changed.TaskAccepted = nil
	}
	defer func() {
		// A job computes something else once its command, environment, or
		// working directory changes, so its recorded result no longer
		// applies. Scheduling settings such as the timeout keep it.
		if !slices.Equal(before.Command, changed.Command) || !slices.Equal(before.Environment, changed.Environment) || before.WorkingDirectory != changed.WorkingDirectory {
			changed.Force = true
			changed.Accepted = false
			changed.TaskAccepted = nil
		}
	}()
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
// requestedRunID, or leaves it alone when requestedRunID is empty.
func restoreSnapshot(paths state.ProjectPaths, requestedRunID string, queue *model.Queue) error {
	if requestedRunID == "" {
		return nil
	}
	runID, err := resolve.RunID(paths, requestedRunID)
	if err != nil {
		return err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	snapshot, err := state.ReadQueueFile(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return fmt.Errorf("failed to load command snapshot: %w", err)
	}
	if len(snapshot.Commands) == 0 {
		return errors.New("command snapshot has no jobs")
	}
	*queue = snapshot
	return nil
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
