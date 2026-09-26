package queueedit

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Run is the source run that jobs are copied from.
type Run struct {
	ID       string
	Snapshot model.Queue
	Results  map[string]model.JobResult
	CWD      string
	// Timestamps returns a job's submitted and finished times in the run.
	Timestamps func(jobID string) (submittedAt, finishedAt string)
}

// Attempt selects one attempt of a job in the source run. Callers check that
// the attempt belongs to the run and exists before copying.
type Attempt struct {
	ID    string
	JobID string
}

// CopyRequest selects the source jobs to copy.
type CopyRequest struct {
	// Selection is "all", "job-id", or a result selection such as "failed".
	Selection string
	// JobIDs are the jobs to copy, with Selection "job-id".
	JobIDs []string
	// Scope, when set, narrows Selection to one stage or matrix.
	Scope model.CommandSelector
	// Attempts are attempts to copy, with Selection "job-id". Selecting an
	// attempt of an array task narrows the copied array to the selected tasks.
	Attempts []Attempt
	// Append keeps the destination queue's jobs; Overwrite replaces them.
	// With neither, a non-empty destination queue is an error.
	Append    bool
	Overwrite bool
}

// Copy returns destination with the selected jobs of source added, and the
// number of jobs copied. project names the destination in error messages and
// newID generates IDs for colliding jobs and copied matrix groups.
func Copy(destination model.Queue, project string, source Run, request CopyRequest, newID func() string) (model.Queue, int, error) {
	if len(request.JobIDs)+len(request.Attempts) > 0 && request.Selection != "job-id" {
		return model.Queue{}, 0, fmt.Errorf("job IDs cannot be combined with selection %q", request.Selection)
	}
	requested := make(map[string]bool, len(request.JobIDs)+len(request.Attempts))
	requestedAttempts := make(map[string]string, len(request.Attempts))
	requestedTasks := make(map[string]map[string]bool)
	for _, attempt := range request.Attempts {
		found := false
		for _, task := range model.QueueToJobs(source.Snapshot.Commands) {
			if task.ID != attempt.JobID {
				continue
			}
			if task.ArrayGroup != "" {
				requested[task.ArrayGroup] = true
				requestedAttempts[task.ArrayGroup+"/"+task.ID] = attempt.ID
				if requestedTasks[task.ArrayGroup] == nil {
					requestedTasks[task.ArrayGroup] = make(map[string]bool)
				}
				requestedTasks[task.ArrayGroup][task.ID] = true
			} else {
				requested[task.ID] = true
				requestedAttempts[task.ID] = attempt.ID
			}
			found = true
			break
		}
		if !found {
			return model.Queue{}, 0, fmt.Errorf("attempt %q job %q not found in run %s", attempt.ID, attempt.JobID, source.ID)
		}
	}
	// A task ID narrows its array to the requested tasks, as an attempt
	// does; the array's own ID copies it whole.
	tasks := make(map[string]model.JobSpec)
	for _, task := range model.QueueToJobs(source.Snapshot.Commands) {
		if task.ArrayGroup != "" {
			tasks[task.ID] = task
		}
	}
	whole := make(map[string]bool, len(request.JobIDs))
	for _, jobID := range request.JobIDs {
		task, isTask := tasks[jobID]
		if !isTask {
			requested[jobID] = true
			whole[jobID] = true
			continue
		}
		requested[task.ArrayGroup] = true
		if requestedTasks[task.ArrayGroup] == nil {
			requestedTasks[task.ArrayGroup] = make(map[string]bool)
		}
		requestedTasks[task.ArrayGroup][task.ID] = true
	}
	for commandID := range whole {
		delete(requestedTasks, commandID)
	}

	inScope := func(model.QueuedCommand) bool { return true }
	if request.Scope.Kinds() > 0 {
		if _, err := model.SelectCommands(source.Snapshot.Commands, request.Scope); err != nil {
			return model.Queue{}, 0, fmt.Errorf("run %s: %w", source.ID, err)
		}
		inScope = request.Scope.Matches
	}
	selected, selectedNames, err := selectCommands(source, request.Selection, inScope, requested, requestedTasks)
	if err != nil {
		return model.Queue{}, 0, err
	}
	keep, err := checkOmittedDependencies(source, selected, selectedNames)
	if err != nil {
		return model.Queue{}, 0, err
	}

	if len(destination.Commands) > 0 && !request.Append && !request.Overwrite {
		return model.Queue{}, 0, fmt.Errorf("project %q has queued jobs; use --append or --overwrite", project)
	}
	model.ClearIncompleteMatrixGroups(selected)
	renameMatrixGroups(selected, newID)
	existingIDs := make(map[string]bool)
	if request.Append {
		for _, command := range destination.Commands {
			existingIDs[command.ID] = true
		}
	}
	for index := range selected {
		command := &selected[index]
		sourceJobID := command.ID
		dependencies := keptNames(command.DependsOn, keep)
		finishedDependencies := keptNames(command.DependsOnFinished, keep)
		// Keep the source job ID so the copied job can still be matched
		// against the source run's results; only reassign on collision.
		if existingIDs[sourceJobID] {
			command.ID = newID()
		}
		existingIDs[command.ID] = true
		command.DependsOn = dependencies
		command.DependsOnFinished = finishedDependencies
		command.Accepted = false
		command.TaskAccepted = nil
		command.TaskForce = nil
		command.Force = request.Append && destination.WorkflowImport
		command.Origin = commandOrigin(source, sourceJobID, command.Array, requestedAttempts)
		if command.Array != nil {
			command.TaskOrigins = taskOrigins(source, sourceJobID, command.Array, requestedAttempts)
		}
	}
	if !request.Append {
		destination.Commands = nil
		destination.WorkflowImport = false
	}
	destination.Commands = append(destination.Commands, selected...)
	if err := model.ValidateQueueDependencies(destination.Commands); err != nil {
		return model.Queue{}, 0, fmt.Errorf("invalid dependencies: %w", err)
	}
	return destination, len(selected), nil
}

func selectCommands(source Run, selection string, inScope func(model.QueuedCommand) bool, requested map[string]bool, requestedTasks map[string]map[string]bool) ([]model.QueuedCommand, map[string]bool, error) {
	selected := make([]model.QueuedCommand, 0, len(source.Snapshot.Commands))
	selectedNames := make(map[string]bool)
	for _, command := range source.Snapshot.Commands {
		result, finished := model.AggregatedJobResult(command.ID, command.Array, source.Results)
		include := false
		switch selection {
		case "all":
			include = true
		case "job-id":
			include = requested[command.ID]
		default:
			include = model.ResultSelectionMatches(selection, finished, result.ExitCode)
		}
		include = include && inScope(command)
		if requested[command.ID] {
			include = true
			delete(requested, command.ID)
		}
		if include {
			if taskIDs := requestedTasks[command.ID]; len(taskIDs) > 0 {
				narrowArrayCommand(&command, taskIDs)
			}
			selectedNames[command.Name] = true
			selected = append(selected, command)
		}
	}
	if len(requested) > 0 {
		missing := make([]string, 0, len(requested))
		for jobID := range requested {
			missing = append(missing, jobID)
		}
		sort.Strings(missing)
		return nil, nil, fmt.Errorf("job IDs not found in run %s: %s", source.ID, strings.Join(missing, ", "))
	}
	if len(selected) == 0 {
		return nil, nil, fmt.Errorf("run %s has no jobs matching selection", source.ID)
	}
	return selected, selectedNames, nil
}

func keptNames(names []string, keep func(string) bool) []string {
	if names == nil {
		return nil
	}
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if keep(name) {
			kept = append(kept, name)
		}
	}
	return kept
}

// checkOmittedDependencies requires every omitted DependsOn prerequisite of a
// copied job to have succeeded, and every omitted DependsOnFinished
// prerequisite to have finished with any result. It returns which dependency
// names copied jobs keep.
//
// A stage dependency stays as long as any stage member is copied: copied
// members keep their Stage, so the name resolves to them, and only the
// omitted members must have succeeded. A matrix base name resolves to all of
// its members; it stays a dependency target only when the whole group is
// copied, otherwise the omitted members must have succeeded and
// ClearIncompleteMatrixGroups rewrites the dependency to the copied members.
func checkOmittedDependencies(source Run, selected []model.QueuedCommand, selectedNames map[string]bool) (func(string) bool, error) {
	commandsByName := make(map[string]model.QueuedCommand, len(source.Snapshot.Commands))
	stages := make(map[string]bool)
	matrixMembers := make(map[string][]model.QueuedCommand)
	for _, command := range source.Snapshot.Commands {
		commandsByName[command.Name] = command
		if command.Stage != "" {
			stages[command.Stage] = true
		}
		if command.Matrix != nil && command.Matrix.BaseName != "" {
			matrixMembers[command.Matrix.BaseName] = append(matrixMembers[command.Matrix.BaseName], command)
		}
	}
	selectedIDs := make(map[string]bool, len(selected))
	keptStages := make(map[string]bool)
	for _, command := range selected {
		selectedIDs[command.ID] = true
		if command.Stage != "" {
			keptStages[command.Stage] = true
		}
	}
	completeMatrices := make(map[string]bool)
	for baseName, members := range matrixMembers {
		complete := true
		for _, member := range members {
			complete = complete && selectedNames[member.Name]
		}
		completeMatrices[baseName] = complete
	}
	succeeded := func(command model.QueuedCommand) bool {
		result, finished := model.AggregatedJobResult(command.ID, command.Array, source.Results)
		return finished && result.ExitCode == 0
	}
	finished := func(command model.QueuedCommand) bool {
		_, finished := model.AggregatedJobResult(command.ID, command.Array, source.Results)
		return finished
	}
	// check requires each omitted prerequisite in dependencies to satisfy
	// ready, whose failure is described by outcome.
	check := func(command model.QueuedCommand, dependencies []string, ready func(model.QueuedCommand) bool, outcome string) error {
		for _, dependency := range dependencies {
			if selectedNames[dependency] || completeMatrices[dependency] {
				continue
			}
			if members, isMatrix := matrixMembers[dependency]; isMatrix {
				for _, member := range members {
					if !selectedNames[member.Name] && !ready(member) {
						return fmt.Errorf("cannot copy job %q: excluded dependency %q did not %s in run %s", command.Name, member.Name, outcome, source.ID)
					}
				}
				continue
			}
			if stages[dependency] {
				for _, candidate := range source.Snapshot.Commands {
					if candidate.Stage == dependency && !selectedIDs[candidate.ID] && !ready(candidate) {
						return fmt.Errorf("cannot copy job %q: excluded dependency %q (stage %q) did not %s in run %s", command.Name, stageMemberLabel(candidate), dependency, outcome, source.ID)
					}
				}
				continue
			}
			if !ready(commandsByName[dependency]) {
				return fmt.Errorf("cannot copy job %q: excluded dependency %q did not %s in run %s", command.Name, dependency, outcome, source.ID)
			}
		}
		return nil
	}
	for _, command := range selected {
		if err := check(command, command.DependsOn, succeeded, "succeed"); err != nil {
			return nil, err
		}
		if err := check(command, command.DependsOnFinished, finished, "finish"); err != nil {
			return nil, err
		}
	}
	keep := func(dependency string) bool {
		return selectedNames[dependency] || keptStages[dependency] || completeMatrices[dependency]
	}
	return keep, nil
}

// renameMatrixGroups gives each copied matrix group a new group ID, so the
// copy is a distinct group from its source.
func renameMatrixGroups(commands []model.QueuedCommand, newID func() string) {
	groupIDs := make(map[string]string)
	for index := range commands {
		if commands[index].Matrix == nil {
			continue
		}
		oldGroupID := commands[index].Matrix.GroupID
		newGroupID, ok := groupIDs[oldGroupID]
		if !ok {
			newGroupID = newID()
			groupIDs[oldGroupID] = newGroupID
		}
		matrix := *commands[index].Matrix
		matrix.GroupID = newGroupID
		commands[index].Matrix = &matrix
	}
}

func stageMemberLabel(command model.QueuedCommand) string {
	if command.Name != "" {
		return command.Name
	}
	return command.ID
}

func narrowArrayCommand(command *model.QueuedCommand, taskIDs map[string]bool) {
	if command.Array == nil {
		return
	}
	tasks := make([]int, 0, len(taskIDs))
	for _, task := range model.ArrayTaskIDs(command.Array) {
		if taskIDs[fmt.Sprintf("%s-%d", command.ID, task)] {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		return
	}
	sort.Ints(tasks)
	command.Array = &model.ArraySpec{First: tasks[0], Last: tasks[len(tasks)-1], Tasks: tasks}
}

func resultStatus(result model.JobResult) string {
	if result.ExitCode == 0 {
		return "success"
	}
	return "failed"
}

func commandOrigin(source Run, jobID string, array *model.ArraySpec, requestedAttempts map[string]string) *model.JobOrigin {
	status := "unfinished"
	attemptID := ""
	if result, finished := model.AggregatedJobResult(jobID, array, source.Results); finished {
		attemptID = result.AttemptID
		status = resultStatus(result)
	}
	if explicitAttemptID, ok := requestedAttempts[jobID]; ok {
		attemptID = explicitAttemptID
	}
	submittedAt, finishedAt := commandTimestamps(source, jobID, array)
	return &model.JobOrigin{RunID: source.ID, JobID: jobID, AttemptID: attemptID, Status: status, CWD: source.CWD, SubmittedAt: submittedAt, FinishedAt: finishedAt}
}

func taskOrigins(source Run, commandID string, array *model.ArraySpec, requestedAttempts map[string]string) map[string]*model.JobOrigin {
	origins := make(map[string]*model.JobOrigin)
	for _, task := range model.ArrayTaskIDs(array) {
		taskID := fmt.Sprintf("%s-%d", commandID, task)
		result, finished := source.Results[taskID]
		if !finished {
			continue
		}
		attemptID := result.AttemptID
		if explicitAttemptID, ok := requestedAttempts[commandID+"/"+taskID]; ok {
			attemptID = explicitAttemptID
		}
		submittedAt, finishedAt := source.Timestamps(taskID)
		origins[taskID] = &model.JobOrigin{
			RunID: source.ID, JobID: taskID, AttemptID: attemptID, Status: resultStatus(result), CWD: source.CWD,
			SubmittedAt: submittedAt, FinishedAt: finishedAt,
		}
	}
	return origins
}

// commandTimestamps returns the times to record on a copied job's Origin. An
// array job's results are stored per task, so it reports the earliest
// submission and latest completion across its tasks.
func commandTimestamps(source Run, id string, array *model.ArraySpec) (string, string) {
	if array == nil {
		return source.Timestamps(id)
	}
	var submittedAt, finishedAt string
	for _, task := range model.ArrayTaskIDs(array) {
		taskSubmitted, taskFinished := source.Timestamps(fmt.Sprintf("%s-%d", id, task))
		if taskSubmitted != "" && (submittedAt == "" || taskSubmitted < submittedAt) {
			submittedAt = taskSubmitted
		}
		if taskFinished != "" && taskFinished > finishedAt {
			finishedAt = taskFinished
		}
	}
	return submittedAt, finishedAt
}
