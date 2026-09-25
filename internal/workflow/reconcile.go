package workflow

import (
	"fmt"
	"reflect"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// SourceStore reads the source runs and attempts that a run-exported
// manifest names. Implementations own filesystem access and path validation.
type SourceStore interface {
	// Run loads a source run.
	Run(runID string) (SourceRun, error)
	// Attempt checks that attemptID of jobID exists in runID and returns the
	// result it records, if finished.
	Attempt(runID, jobID, attemptID string, command []string) (result model.JobResult, finished bool, err error)
	// AttemptTimestamps returns an attempt's own submitted and finished times.
	AttemptTimestamps(runID, jobID, attemptID string) (submittedAt, finishedAt string)
}

// RemovedJob is a source job that the manifest no longer describes.
type RemovedJob struct {
	RunID   string   `json:"run_id"`
	JobID   string   `json:"job_id"`
	Name    string   `json:"name,omitempty"`
	Command []string `json:"command"`
}

type sourceCatalog struct {
	store   SourceStore
	runs    map[string]*SourceRun
	ordered []*SourceRun
	allowed map[string]bool
}

type sourceLeaf struct {
	run      *SourceRun
	command  model.QueuedCommand
	jobID    string
	result   model.JobResult
	finished bool
}

// Reconcile writes explicit carry, force, and acceptance dispositions into a
// queue compiled from a run-exported manifest, by matching its jobs to their
// source attempts. It also lists exported source jobs that the manifest no
// longer describes. A manifest without a source is returned unchanged.
func Reconcile(manifest Manifest, queue model.Queue, store SourceStore) (model.Queue, []RemovedJob, error) {
	if manifest.Source == nil {
		return queue, nil, nil
	}
	catalog, err := loadSourceCatalog(store, *manifest.Source)
	if err != nil {
		return model.Queue{}, nil, err
	}
	queue.WorkflowImport = true
	cursor := 0
	for _, job := range manifest.Jobs {
		count, err := reconcileJob(job, queue.Commands[cursor:], catalog)
		if err != nil {
			return model.Queue{}, nil, err
		}
		cursor += count
	}
	return queue, catalog.removedJobs(manifest, queue), nil
}

// removedJobs lists exported source commands that no manifest job refers to.
// A source command is kept when the manifest names one of its attempts, when
// another member of its matrix group is kept, when its name is still queued,
// or, for an unnamed command without attempts, when an identical definition
// is still queued. The result is informational; import never infers identity
// from it.
func (catalog *sourceCatalog) removedJobs(manifest Manifest, queue model.Queue) []RemovedJob {
	attempts := make(map[string]bool)
	for _, job := range manifest.Jobs {
		attempts[job.AttemptID] = true
		for _, instance := range job.Instances {
			attempts[instance.AttemptID] = true
		}
	}
	delete(attempts, "")
	names := make(map[string]bool, len(queue.Commands))
	for _, command := range queue.Commands {
		if command.Name != "" {
			names[command.Name] = true
		}
	}
	sources := catalog.exportedCommands()
	kept := make([]bool, len(sources))
	keptGroups := make(map[string]bool)
	for index, source := range sources {
		for _, attemptID := range leafAttemptIDs(source) {
			if attempts[attemptID] {
				kept[index] = true
			}
		}
		if kept[index] && source.command.Matrix != nil {
			keptGroups[source.run.ID+"\x00"+source.command.Matrix.GroupID] = true
		}
	}
	removed := make([]RemovedJob, 0)
	for index, source := range sources {
		command := source.command
		switch {
		case kept[index]:
		case command.Matrix != nil && keptGroups[source.run.ID+"\x00"+command.Matrix.GroupID]:
		case command.Name != "" && names[command.Name]:
		case command.Name == "" && len(leafAttemptIDs(source)) == 0 && queueHasEquivalentCommand(queue, command):
		default:
			removed = append(removed, RemovedJob{RunID: source.run.ID, JobID: command.ID, Name: command.Name, Command: command.Command})
		}
	}
	return removed
}

// exportedCommands returns the latest listed snapshot of each source command
// ID in first-appearance order, matching MergeRuns.
func (catalog *sourceCatalog) exportedCommands() []sourceLeaf {
	seen := make(map[string]bool)
	sources := make([]sourceLeaf, 0)
	for _, run := range catalog.ordered {
		for _, command := range run.Queue.Commands {
			if seen[command.ID] {
				continue
			}
			seen[command.ID] = true
			sources = append(sources, catalog.listedCommandRun(sourceLeaf{run: run, command: command}))
		}
	}
	return sources
}

func leafAttemptIDs(source sourceLeaf) []string {
	attempts := make([]string, 0)
	for _, leafID := range commandLeafIDs(source.command) {
		if result, ok := source.run.results[leafID]; ok && result.AttemptID != "" {
			attempts = append(attempts, result.AttemptID)
		}
	}
	return attempts
}

func queueHasEquivalentCommand(queue model.Queue, command model.QueuedCommand) bool {
	for _, candidate := range queue.Commands {
		if EquivalentCommand(candidate, command) {
			return true
		}
	}
	return false
}

func reconcileJob(job Job, remaining []model.QueuedCommand, catalog *sourceCatalog) (int, error) {
	count, err := jobCommandCount(job)
	if err != nil {
		return 0, err
	}
	if count > len(remaining) {
		return 0, fmt.Errorf("job %q expansion exceeds compiled queue", job.Name)
	}
	commands := remaining[:count]
	if err := validateInstances(job, commands); err != nil {
		return 0, err
	}
	if err := validateAttempts(job, catalog); err != nil {
		return 0, err
	}
	anchorAttempt := anchorAttempt(job)
	if anchorAttempt == "" {
		forceCommands(commands)
		return count, nil
	}
	anchor, err := catalog.resolveAttempt(anchorAttempt)
	if err != nil {
		return 0, err
	}
	for index := range commands {
		if err := reconcileCommand(&commands[index], job, anchor, catalog); err != nil {
			return 0, err
		}
	}
	return count, nil
}

func validateAttempts(job Job, catalog *sourceCatalog) error {
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

func anchorAttempt(job Job) string {
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

func reconcileCommand(destination *model.QueuedCommand, job Job, anchor sourceLeaf, catalog *sourceCatalog) error {
	source, ok := catalog.matchCommand(anchor, *destination)
	if !ok || !EquivalentCommand(*destination, source.command) {
		destination.Force = true
		destination.Accepted = false
		destination.TaskAccepted = nil
		return nil
	}
	return reconcileCommandLeaves(destination, source, job, catalog)
}

func validateInstances(job Job, commands []model.QueuedCommand) error {
	seen := make(map[string]bool, len(job.Instances))
	for _, instance := range job.Instances {
		key, matches := instanceKey(instance, commands)
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

func instanceKey(instance Instance, commands []model.QueuedCommand) (string, int) {
	key := ""
	matches := 0
	for index := range commands {
		candidate, ok := instanceCommandKey(instance, commands[index])
		if ok {
			key = candidate
			matches++
		}
	}
	return key, matches
}

func instanceCommandKey(instance Instance, command model.QueuedCommand) (string, bool) {
	if !reflect.DeepEqual(instance.Matrix, commandMatrixValues(command.Matrix)) {
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

func jobCommandCount(job Job) (int, error) {
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

func forceCommands(commands []model.QueuedCommand) {
	for index := range commands {
		commands[index].Force = true
		commands[index].Accepted = false
		commands[index].TaskAccepted = nil
	}
}

func loadSourceCatalog(store SourceStore, source Source) (*sourceCatalog, error) {
	catalog := &sourceCatalog{store: store, runs: make(map[string]*SourceRun), allowed: make(map[string]bool)}
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

func (catalog *sourceCatalog) loadRun(runID string) (*SourceRun, error) {
	if run := catalog.runs[runID]; run != nil {
		return run, nil
	}
	loaded, err := catalog.store.Run(runID)
	if err != nil {
		return nil, err
	}
	loaded.Queue = FlattenQueueDefaults(loaded.Queue)
	loaded.results = model.ResultsByID(loaded.Summary.Results)
	run := &loaded
	catalog.runs[runID] = run
	return run, nil
}

// allowRunQueue records the jobs a listed run can reach: its own jobs, the
// origins it carried results from, and the runs that produced its results.
func (catalog *sourceCatalog) allowRunQueue(run *SourceRun) {
	for _, command := range run.Queue.Commands {
		for _, jobID := range append([]string{command.ID}, commandLeafIDs(command)...) {
			catalog.allowed[sourceKey(run.ID, jobID)] = true
		}
		if command.Origin != nil {
			catalog.allowed[sourceKey(command.Origin.RunID, command.Origin.JobID)] = true
		}
		for _, origin := range command.TaskOrigins {
			catalog.allowed[sourceKey(origin.RunID, origin.JobID)] = true
		}
	}
	for _, result := range run.Summary.Results {
		if result.AttemptID == "" {
			continue
		}
		if payload, err := state.DecodeAttemptID(result.AttemptID); err == nil {
			catalog.allowed[sourceKey(payload.RunID, payload.JobID)] = true
		}
	}
}

func sourceKey(runID, jobID string) string {
	return runID + "\x00" + jobID
}

func (catalog *sourceCatalog) resolveAttempt(attemptID string) (sourceLeaf, error) {
	payload, err := state.DecodeAttemptID(attemptID)
	if err != nil {
		return sourceLeaf{}, err
	}
	if !catalog.allowed[sourceKey(payload.RunID, payload.JobID)] {
		return sourceLeaf{}, fmt.Errorf("attempt %q is not reachable from the exported source runs", attemptID)
	}
	run, err := catalog.loadRun(payload.RunID)
	if err != nil {
		return sourceLeaf{}, err
	}
	command, ok := sourceCommandForJobID(run.Queue.Commands, payload.JobID)
	if !ok {
		return sourceLeaf{}, fmt.Errorf("job %q from attempt %q is not in source snapshot", payload.JobID, attemptID)
	}
	attemptResult, attemptFinished, err := catalog.store.Attempt(run.ID, payload.JobID, attemptID, command.Command)
	if err != nil {
		return sourceLeaf{}, err
	}
	result, finished := run.results[payload.JobID]
	if !finished || result.AttemptID != attemptID {
		if !attemptFinished {
			return sourceLeaf{}, fmt.Errorf("attempt %q has no completed result", attemptID)
		}
		result, finished = attemptResult, true
	}
	return sourceLeaf{run: run, command: command, jobID: payload.JobID, result: result, finished: finished}, nil
}

func sourceCommandForJobID(commands []model.QueuedCommand, jobID string) (model.QueuedCommand, bool) {
	for _, command := range commands {
		if command.ID == jobID {
			return command, true
		}
		for _, leafID := range commandLeafIDs(command) {
			if leafID == jobID {
				return command, true
			}
		}
	}
	return model.QueuedCommand{}, false
}

func (catalog *sourceCatalog) matchCommand(anchor sourceLeaf, destination model.QueuedCommand) (sourceLeaf, bool) {
	if destination.Matrix == nil || anchor.command.Matrix == nil {
		return sourceLeaf{run: anchor.run, command: anchor.command}, true
	}
	var selected sourceLeaf
	selectedTimestamp := ""
	for _, run := range catalog.ordered {
		for _, command := range run.Queue.Commands {
			if command.Matrix == nil || command.Matrix.GroupID != anchor.command.Matrix.GroupID || !reflect.DeepEqual(command.Matrix.Values, destination.Matrix.Values) {
				continue
			}
			timestamp := run.CommandTimestamp(command)
			if selected.run == nil || newerSnapshot(timestamp, run.ID, selectedTimestamp, selected.run.ID) {
				selected = sourceLeaf{run: run, command: command}
				selectedTimestamp = timestamp
			}
		}
	}
	return selected, selected.run != nil
}

func reconcileCommandLeaves(destination *model.QueuedCommand, source sourceLeaf, manifestJob Job, catalog *sourceCatalog) error {
	destination.ID = source.command.ID
	source = catalog.listedCommandRun(source)
	if destination.Array == nil {
		// A matrix group has no single attempt; its job-level attempt only
		// anchors the group, so members fall back to the listed source run.
		attemptID := ""
		if destination.Matrix == nil {
			attemptID = manifestJob.AttemptID
		}
		if instance := findInstance(manifestJob, destination.Matrix, nil); instance != nil && instance.AttemptID != "" {
			attemptID = instance.AttemptID
		}
		leaf, err := catalog.sourceLeafForCommand(source, source.command.ID, attemptID)
		if err != nil {
			return err
		}
		status := desiredStatus(manifestJob, destination.Matrix, nil, leaf.result)
		catalog.applyLeaf(destination, "", leaf, status)
		return nil
	}
	for _, task := range model.ArrayTaskIDs(destination.Array) {
		destinationID := fmt.Sprintf("%s-%d", destination.ID, task)
		sourceID := fmt.Sprintf("%s-%d", source.command.ID, task)
		instance := findInstance(manifestJob, destination.Matrix, &task)
		attemptID := ""
		if instance != nil {
			attemptID = instance.AttemptID
		}
		leaf, err := catalog.sourceLeafForCommand(source, sourceID, attemptID)
		if err != nil {
			return err
		}
		status := desiredStatus(manifestJob, destination.Matrix, &task, leaf.result)
		catalog.applyLeaf(destination, destinationID, leaf, status)
	}
	return nil
}

func (catalog *sourceCatalog) sourceLeafForCommand(source sourceLeaf, jobID, attemptID string) (sourceLeaf, error) {
	if attemptID != "" {
		leaf, err := catalog.resolveAttempt(attemptID)
		if err != nil {
			return sourceLeaf{}, err
		}
		if leaf.jobID != jobID {
			return sourceLeaf{}, fmt.Errorf("attempt %q belongs to job %q, not %q", attemptID, leaf.jobID, jobID)
		}
		return leaf, nil
	}
	result, ok := source.run.results[jobID]
	if ok && result.AttemptID != "" {
		// A result carried into the listed run keeps the attempt ID of the run
		// that produced it; resolve that run so Origin names the real attempt.
		payload, err := state.DecodeAttemptID(result.AttemptID)
		if err != nil {
			return sourceLeaf{}, fmt.Errorf("source result %q has invalid attempt: %w", jobID, err)
		}
		if payload.RunID != source.run.ID {
			return catalog.resolveAttempt(result.AttemptID)
		}
	}
	return sourceLeaf{run: source.run, command: source.command, jobID: jobID, result: result, finished: ok}, nil
}

// listedCommandRun selects the listed source run whose snapshot supplied the
// command to export, using the same latest-run rule as MergeRuns. Leaves
// without an explicit manifest attempt are recovered from that run rather
// than from the run that the job-level anchor attempt happens to point to.
func (catalog *sourceCatalog) listedCommandRun(source sourceLeaf) sourceLeaf {
	selected := source
	selectedTimestamp := ""
	found := false
	for _, run := range catalog.ordered {
		for _, command := range run.Queue.Commands {
			if command.ID != source.command.ID {
				continue
			}
			timestamp := run.CommandTimestamp(command)
			if !found || newerSnapshot(timestamp, run.ID, selectedTimestamp, selected.run.ID) {
				selected = sourceLeaf{run: run, command: command}
				selectedTimestamp = timestamp
				found = true
			}
		}
	}
	return selected
}

func findInstance(job Job, matrix *model.MatrixSpec, task *int) *Instance {
	for index := range job.Instances {
		instance := &job.Instances[index]
		if !reflect.DeepEqual(instance.Matrix, commandMatrixValues(matrix)) {
			continue
		}
		if instance.Task == nil && task == nil || instance.Task != nil && task != nil && *instance.Task == *task {
			return instance
		}
	}
	return nil
}

func commandMatrixValues(matrix *model.MatrixSpec) map[string]string {
	if matrix == nil {
		return nil
	}
	return matrixValuesMap(matrix.Values)
}

func desiredStatus(job Job, matrix *model.MatrixSpec, task *int, result model.JobResult) string {
	if job.Status == "success" {
		return "success"
	}
	if instance := findInstance(job, matrix, task); instance != nil {
		return instance.Status
	}
	if len(job.Instances) == 0 && job.Status != "" {
		return job.Status
	}
	return resultStatus(result, true)
}

func (catalog *sourceCatalog) applyLeaf(command *model.QueuedCommand, destinationID string, source sourceLeaf, desiredStatus string) {
	actualStatus := resultStatus(source.result, source.finished)
	submittedAt, finishedAt := catalog.leafTimestamps(source)
	origin := &model.JobOrigin{
		RunID: source.run.ID, JobID: source.jobID, AttemptID: source.result.AttemptID,
		Status: actualStatus, CWD: source.run.CWD, SubmittedAt: submittedAt, FinishedAt: finishedAt,
	}
	if destinationID == "" {
		command.Origin = origin
		command.Force = desiredStatus != "success"
		command.Accepted = desiredStatus == "success" && actualStatus != "success"
		return
	}
	if command.TaskOrigins == nil {
		command.TaskOrigins = make(map[string]*model.JobOrigin)
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

func (catalog *sourceCatalog) leafTimestamps(source sourceLeaf) (string, string) {
	if source.result.AttemptID != "" {
		return catalog.store.AttemptTimestamps(source.run.ID, source.jobID, source.result.AttemptID)
	}
	return source.run.JobTimestamps(source.jobID)
}
