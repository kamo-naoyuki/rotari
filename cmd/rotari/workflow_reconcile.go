package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

type workflowSourceRun struct {
	id      string
	dir     string
	queue   Queue
	summary RunSummary
	results map[string]JobResult
	cwd     string
}

type workflowSourceCatalog struct {
	paths   pathSet
	runs    map[string]*workflowSourceRun
	ordered []*workflowSourceRun
	allowed map[string]bool
}

type workflowSourceLeaf struct {
	run      *workflowSourceRun
	command  QueuedCommand
	jobID    string
	result   JobResult
	finished bool
}

func reconcileWorkflowManifest(baseDir string, manifest workflow.Manifest, queue Queue) (Queue, error) {
	if manifest.Source == nil {
		return queue, nil
	}
	catalog, err := loadWorkflowSourceCatalog(baseDir, *manifest.Source)
	if err != nil {
		return Queue{}, err
	}
	queue.WorkflowImport = true
	cursor := 0
	for _, job := range manifest.Jobs {
		count, err := reconcileWorkflowJob(job, queue.Commands[cursor:], catalog)
		if err != nil {
			return Queue{}, err
		}
		cursor += count
	}
	if err := validateQueueJobs(queue); err != nil {
		return Queue{}, err
	}
	return queue, nil
}

func reconcileWorkflowJob(job workflow.Job, remaining []QueuedCommand, catalog *workflowSourceCatalog) (int, error) {
	count, err := workflowJobCommandCount(job)
	if err != nil {
		return 0, err
	}
	if count > len(remaining) {
		return 0, fmt.Errorf("job %q expansion exceeds compiled queue", job.Name)
	}
	commands := remaining[:count]
	if err := validateWorkflowInstances(job, commands); err != nil {
		return 0, err
	}
	if err := validateWorkflowAttempts(job, catalog); err != nil {
		return 0, err
	}
	anchorAttempt := workflowAnchorAttempt(job)
	if anchorAttempt == "" {
		forceWorkflowCommands(commands)
		return count, nil
	}
	anchor, err := catalog.resolveAttempt(anchorAttempt)
	if err != nil {
		return 0, err
	}
	for index := range commands {
		if err := reconcileWorkflowCommand(&commands[index], job, anchor, catalog); err != nil {
			return 0, err
		}
	}
	return count, nil
}

func validateWorkflowAttempts(job workflow.Job, catalog *workflowSourceCatalog) error {
	if job.AttemptID != "" {
		if _, err := catalog.resolveAttempt(job.AttemptID); err != nil {
			return err
		}
	}
	for _, instance := range job.Instances {
		if instance.AttemptID == "" {
			continue
		}
		if _, err := catalog.resolveAttempt(instance.AttemptID); err != nil {
			return err
		}
	}
	return nil
}

func workflowAnchorAttempt(job workflow.Job) string {
	if job.AttemptID != "" {
		return job.AttemptID
	}
	for _, instance := range job.Instances {
		if instance.AttemptID != "" {
			return instance.AttemptID
		}
	}
	return ""
}

func reconcileWorkflowCommand(destination *QueuedCommand, job workflow.Job, anchor workflowSourceLeaf, catalog *workflowSourceCatalog) error {
	source, ok := catalog.matchCommand(anchor, *destination)
	if !ok || !workflow.EquivalentCommand(*destination, source.command) {
		destination.Force = true
		destination.Accepted = false
		destination.TaskAccepted = nil
		return nil
	}
	return reconcileCommandLeaves(destination, source, job, catalog)
}

func validateWorkflowInstances(job workflow.Job, commands []QueuedCommand) error {
	seen := make(map[string]bool, len(job.Instances))
	for _, instance := range job.Instances {
		key, matches := workflowInstanceKey(instance, commands)
		if matches != 1 {
			return fmt.Errorf("job %q instance does not identify exactly one matrix/array leaf", job.Name)
		}
		if seen[key] {
			return fmt.Errorf("job %q has a duplicate instance", job.Name)
		}
		seen[key] = true
	}
	return nil
}

func workflowInstanceKey(instance workflow.Instance, commands []QueuedCommand) (string, int) {
	key := ""
	matches := 0
	for index := range commands {
		candidate, ok := workflowInstanceCommandKey(instance, commands[index])
		if ok {
			key = candidate
			matches++
		}
	}
	return key, matches
}

func workflowInstanceCommandKey(instance workflow.Instance, command QueuedCommand) (string, bool) {
	if !reflect.DeepEqual(instance.Matrix, matrixValuesMapForReconcile(command.Matrix)) {
		return "", false
	}
	if command.Array == nil {
		return command.ID, instance.Task == nil
	}
	if instance.Task == nil {
		return "", false
	}
	for _, task := range model.ArrayTaskIDs(command.Array) {
		if task == *instance.Task {
			return fmt.Sprintf("%s-%d", command.ID, task), true
		}
	}
	return "", false
}

func workflowJobCommandCount(job workflow.Job) (int, error) {
	count := 1
	for _, value := range job.Matrix {
		dimension, err := model.ParseMatrixDimension(value)
		if err != nil {
			return 0, err
		}
		count *= len(dimension.Values)
	}
	return count, nil
}

func forceWorkflowCommands(commands []QueuedCommand) {
	for index := range commands {
		commands[index].Force = true
		commands[index].Accepted = false
		commands[index].TaskAccepted = nil
	}
}

func loadWorkflowSourceCatalog(baseDir string, source workflow.Source) (*workflowSourceCatalog, error) {
	paths, err := resolvePaths(baseDir, source.Project)
	if err != nil {
		return nil, err
	}
	catalog := &workflowSourceCatalog{paths: paths, runs: make(map[string]*workflowSourceRun), allowed: make(map[string]bool)}
	seen := make(map[string]bool)
	for _, runID := range source.RunIDs {
		if seen[runID] {
			return nil, fmt.Errorf("duplicate source run ID %q", runID)
		}
		seen[runID] = true
		run, err := catalog.loadRun(runID)
		if err != nil {
			return nil, err
		}
		catalog.ordered = append(catalog.ordered, run)
		catalog.allowRunQueue(run)
	}
	return catalog, nil
}

func (catalog *workflowSourceCatalog) loadRun(runID string) (*workflowSourceRun, error) {
	if run := catalog.runs[runID]; run != nil {
		return run, nil
	}
	if !state.IsValidPathElement(runID) {
		return nil, fmt.Errorf("invalid source run ID %q", runID)
	}
	runDir := filepath.Join(catalog.paths.RunsDir, runID)
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to load source run %q commands: %w", runID, err)
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to load source run %q summary: %w", runID, err)
	}
	cwd := ""
	if context, contextErr := state.LoadContext(jsonStore(), runDir); contextErr == nil {
		cwd = context.CWD
	}
	run := &workflowSourceRun{id: runID, dir: runDir, queue: flattenQueueDefaults(queue), summary: summary, results: model.ResultsByID(summary.Results), cwd: cwd}
	catalog.runs[runID] = run
	return run, nil
}

func (catalog *workflowSourceCatalog) allowRunQueue(run *workflowSourceRun) {
	for _, command := range run.queue.Commands {
		catalog.allowed[workflowSourceKey(run.id, command.ID)] = true
		for _, task := range model.ArrayTaskIDs(command.Array) {
			catalog.allowed[workflowSourceKey(run.id, fmt.Sprintf("%s-%d", command.ID, task))] = true
		}
		if command.Origin != nil {
			catalog.allowed[workflowSourceKey(command.Origin.RunID, command.Origin.JobID)] = true
		}
		for _, origin := range command.TaskOrigins {
			catalog.allowed[workflowSourceKey(origin.RunID, origin.JobID)] = true
		}
	}
	for _, result := range run.results {
		if result.AttemptID == "" {
			continue
		}
		if payload, err := state.DecodeAttemptID(result.AttemptID); err == nil {
			catalog.allowed[workflowSourceKey(payload.RunID, payload.JobID)] = true
		}
	}
}

func workflowSourceKey(runID, jobID string) string {
	return runID + "\x00" + jobID
}

func (catalog *workflowSourceCatalog) resolveAttempt(attemptID string) (workflowSourceLeaf, error) {
	payload, err := state.DecodeAttemptID(attemptID)
	if err != nil {
		return workflowSourceLeaf{}, err
	}
	if !catalog.allowed[workflowSourceKey(payload.RunID, payload.JobID)] {
		return workflowSourceLeaf{}, fmt.Errorf("attempt %q is not reachable from the exported source runs", attemptID)
	}
	run, err := catalog.loadRun(payload.RunID)
	if err != nil {
		return workflowSourceLeaf{}, err
	}
	command, ok := sourceCommandForJobID(run.queue.Commands, payload.JobID)
	if !ok {
		return workflowSourceLeaf{}, fmt.Errorf("job %q from attempt %q is not in source snapshot", payload.JobID, attemptID)
	}
	attemptDir, err := state.SpecificAttemptJobDir(run.dir, payload.JobID, attemptID)
	if err != nil {
		return workflowSourceLeaf{}, err
	}
	if info, err := os.Stat(attemptDir); err != nil || !info.IsDir() {
		return workflowSourceLeaf{}, fmt.Errorf("attempt %q not found", attemptID)
	}
	result, finished := run.results[payload.JobID]
	if !finished || result.AttemptID != attemptID {
		if local, ok := state.LoadLocalJobResult(attemptDir, JobSpec{ID: payload.JobID, Command: command.Command}); ok {
			local.AttemptID = attemptID
			return workflowSourceLeaf{run: run, command: command, jobID: payload.JobID, result: local, finished: true}, nil
		}
		status, ok := executor.LoadWrapperStatus(jsonStore(), filepath.Join(attemptDir, statusJSONName))
		if !ok || (status.Phase != "finished" && status.Phase != "failed" && status.Phase != "cancelled") {
			return workflowSourceLeaf{}, fmt.Errorf("attempt %q has no completed result", attemptID)
		}
		result = JobResult{ID: payload.JobID, AttemptID: attemptID, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}
		finished = true
	}
	return workflowSourceLeaf{run: run, command: command, jobID: payload.JobID, result: result, finished: finished}, nil
}

func sourceCommandForJobID(commands []QueuedCommand, jobID string) (QueuedCommand, bool) {
	for _, command := range commands {
		if command.ID == jobID {
			return command, true
		}
		for _, task := range model.ArrayTaskIDs(command.Array) {
			if fmt.Sprintf("%s-%d", command.ID, task) == jobID {
				return command, true
			}
		}
	}
	return QueuedCommand{}, false
}

func (catalog *workflowSourceCatalog) matchCommand(anchor workflowSourceLeaf, destination QueuedCommand) (workflowSourceLeaf, bool) {
	if destination.Matrix == nil || anchor.command.Matrix == nil {
		return workflowSourceLeaf{run: anchor.run, command: anchor.command}, true
	}
	var selected workflowSourceLeaf
	selectedTimestamp := ""
	for _, run := range catalog.ordered {
		for _, command := range run.queue.Commands {
			if command.Matrix == nil || command.Matrix.GroupID != anchor.command.Matrix.GroupID || !reflect.DeepEqual(command.Matrix.Values, destination.Matrix.Values) {
				continue
			}
			timestamp := exportCommandTimestamp(exportRun{ID: run.id, Dir: run.dir, Queue: run.queue, Summary: run.summary}, command)
			if selected.run == nil || timestamp > selectedTimestamp || timestamp == selectedTimestamp && run.id > selected.run.id {
				selected = workflowSourceLeaf{run: run, command: command}
				selectedTimestamp = timestamp
			}
		}
	}
	return selected, selected.run != nil
}

func reconcileCommandLeaves(destination *QueuedCommand, source workflowSourceLeaf, manifestJob workflow.Job, catalog *workflowSourceCatalog) error {
	destination.ID = source.command.ID
	source = catalog.listedCommandRun(source)
	if destination.Array == nil {
		// A matrix group has no single attempt; its job-level attempt only
		// anchors the group, so members fall back to the listed source run.
		attemptID := ""
		if destination.Matrix == nil {
			attemptID = manifestJob.AttemptID
		}
		if instance := workflowInstance(manifestJob, destination.Matrix, nil); instance != nil && instance.AttemptID != "" {
			attemptID = instance.AttemptID
		}
		leaf, err := sourceLeafForCommand(catalog, source, source.command.ID, attemptID)
		if err != nil {
			return err
		}
		status := desiredWorkflowStatus(manifestJob, destination.Matrix, nil, leaf.result)
		applyWorkflowLeaf(destination, "", leaf, status)
		return nil
	}
	for _, task := range model.ArrayTaskIDs(destination.Array) {
		destinationID := fmt.Sprintf("%s-%d", destination.ID, task)
		sourceID := fmt.Sprintf("%s-%d", source.command.ID, task)
		instance := workflowInstance(manifestJob, destination.Matrix, &task)
		attemptID := ""
		if instance != nil {
			attemptID = instance.AttemptID
		}
		leaf, err := sourceLeafForCommand(catalog, source, sourceID, attemptID)
		if err != nil {
			return err
		}
		status := desiredWorkflowStatus(manifestJob, destination.Matrix, &task, leaf.result)
		applyWorkflowLeaf(destination, destinationID, leaf, status)
	}
	return nil
}

func sourceLeafForCommand(catalog *workflowSourceCatalog, source workflowSourceLeaf, jobID, attemptID string) (workflowSourceLeaf, error) {
	if attemptID != "" {
		leaf, err := catalog.resolveAttempt(attemptID)
		if err != nil {
			return workflowSourceLeaf{}, err
		}
		if leaf.jobID != jobID {
			return workflowSourceLeaf{}, fmt.Errorf("attempt %q belongs to job %q, not %q", attemptID, leaf.jobID, jobID)
		}
		return leaf, nil
	}
	result, ok := source.run.results[jobID]
	if ok && result.AttemptID != "" {
		// A result carried into the listed run keeps the attempt ID of the run
		// that produced it; resolve that run so Origin names the real attempt.
		payload, err := state.DecodeAttemptID(result.AttemptID)
		if err != nil {
			return workflowSourceLeaf{}, fmt.Errorf("source result %q has invalid attempt: %w", jobID, err)
		}
		if payload.RunID != source.run.id {
			return catalog.resolveAttempt(result.AttemptID)
		}
	}
	return workflowSourceLeaf{run: source.run, command: source.command, jobID: jobID, result: result, finished: ok}, nil
}

// listedCommandRun selects the listed source run whose snapshot supplied the
// command to export, using the same latest-run rule as export. Leaves without
// an explicit manifest attempt are recovered from that run rather than from
// the run that the job-level anchor attempt happens to point to.
func (catalog *workflowSourceCatalog) listedCommandRun(source workflowSourceLeaf) workflowSourceLeaf {
	selected := source
	selectedTimestamp := ""
	found := false
	for _, run := range catalog.ordered {
		for _, command := range run.queue.Commands {
			if command.ID != source.command.ID {
				continue
			}
			timestamp := exportCommandTimestamp(exportRun{ID: run.id, Dir: run.dir, Queue: run.queue, Summary: run.summary}, command)
			if !found || timestamp > selectedTimestamp || timestamp == selectedTimestamp && run.id > selected.run.id {
				selected = workflowSourceLeaf{run: run, command: command}
				selectedTimestamp = timestamp
				found = true
			}
		}
	}
	return selected
}

func workflowInstance(job workflow.Job, matrix *model.MatrixSpec, task *int) *workflow.Instance {
	for index := range job.Instances {
		instance := &job.Instances[index]
		if !reflect.DeepEqual(instance.Matrix, matrixValuesMapForReconcile(matrix)) {
			continue
		}
		if instance.Task == nil && task == nil || instance.Task != nil && task != nil && *instance.Task == *task {
			return instance
		}
	}
	return nil
}

func matrixValuesMapForReconcile(matrix *model.MatrixSpec) map[string]string {
	if matrix == nil {
		return nil
	}
	values := make(map[string]string, len(matrix.Values))
	for _, value := range matrix.Values {
		values[value.Name] = value.Value
	}
	return values
}

func desiredWorkflowStatus(job workflow.Job, matrix *model.MatrixSpec, task *int, result JobResult) string {
	if job.Status == "success" {
		return "success"
	}
	if instance := workflowInstance(job, matrix, task); instance != nil {
		return instance.Status
	}
	if len(job.Instances) == 0 && job.Status != "" {
		return job.Status
	}
	return workflowResultStatus(result)
}

func applyWorkflowLeaf(command *QueuedCommand, destinationID string, source workflowSourceLeaf, desiredStatus string) {
	actualStatus := "unfinished"
	if source.finished {
		actualStatus = workflowResultStatus(source.result)
	}
	submittedAt, finishedAt := workflowSourceTimestamps(source)
	origin := &JobOrigin{
		RunID: source.run.id, JobID: source.jobID, AttemptID: source.result.AttemptID,
		Status: actualStatus, CWD: source.run.cwd, SubmittedAt: submittedAt, FinishedAt: finishedAt,
	}
	if destinationID == "" {
		command.Origin = origin
		command.Force = desiredStatus != "success"
		command.Accepted = desiredStatus == "success" && actualStatus != "success"
		return
	}
	if command.TaskOrigins == nil {
		command.TaskOrigins = make(map[string]*JobOrigin)
	}
	command.TaskOrigins[destinationID] = origin
	if desiredStatus != "success" {
		if command.TaskForce == nil {
			command.TaskForce = make(map[string]bool)
		}
		command.TaskForce[destinationID] = true
	}
	if desiredStatus == "success" && actualStatus != "success" {
		if command.TaskAccepted == nil {
			command.TaskAccepted = make(map[string]bool)
		}
		command.TaskAccepted[destinationID] = true
	}
}

func workflowResultStatus(result JobResult) string {
	if result.ExitCode == 0 {
		return "success"
	}
	errorText := strings.ToLower(strings.TrimSpace(result.Error))
	if errorText == "cancelled" || errorText == "canceled" || strings.HasPrefix(errorText, "cancelled ") || strings.HasPrefix(errorText, "canceled ") {
		return "cancelled"
	}
	return "failed"
}

func workflowSourceTimestamps(source workflowSourceLeaf) (string, string) {
	if source.result.AttemptID != "" {
		if attemptDir, err := state.SpecificAttemptJobDir(source.run.dir, source.jobID, source.result.AttemptID); err == nil {
			return state.ReadAttemptTimestamp(attemptDir, stateFileSubmittedAt), state.ReadAttemptTimestamp(attemptDir, stateFileFinishedAt)
		}
	}
	return state.ReadJobTimestamp(source.run.dir, source.jobID, stateFileSubmittedAt), state.ReadJobTimestamp(source.run.dir, source.jobID, stateFileFinishedAt)
}
