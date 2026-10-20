package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type RepairAssessment struct {
	Complete   bool                      `json:"complete"`
	Reason     string                    `json:"reason"`
	Executions []model.RecoveryExecution `json:"executions,omitempty"`
}

// incompleteRepair is a readable but untrustworthy completion record, not an
// I/O failure. It permits ordinary release, never metadata finalization.
type incompleteRepair string

func (err incompleteRepair) Error() string { return string(err) }

type repairInventory struct {
	paths      state.ProjectPaths
	store      state.Store
	executions []model.RecoveryExecution
	seen       map[string]bool
}

func assessRepair(paths state.ProjectPaths, runID string, meta model.Meta, store state.Store) (RepairAssessment, model.RunSummary, error) {
	assessment := RepairAssessment{Reason: "no complete summary is available; only release the project"}
	runDir := filepath.Join(paths.RunsDir, runID)
	files, err := state.LoadRunSnapshot(runDir)
	if err != nil {
		return assessment, model.RunSummary{}, err
	}
	if errors.Is(files.SummaryErr, os.ErrNotExist) {
		return assessment, model.RunSummary{}, nil
	}
	if files.SummaryErr != nil {
		return assessment, model.RunSummary{}, fmt.Errorf("read repair summary: %w", files.SummaryErr)
	}
	if meta.LastRunID != runID || (meta.Phase != "running" && meta.Phase != "cancelling") {
		assessment.Reason = "metadata does not name this run as running or cancelling"
		return assessment, files.Summary, nil
	}
	inventory := repairInventory{paths: paths, store: store, seen: make(map[string]bool)}
	if err := inventory.validate(runID, files); err != nil {
		var incomplete incompleteRepair
		if !errors.As(err, &incomplete) {
			return assessment, files.Summary, err
		}
		assessment.Reason = incomplete.Error() + "; only release the project"
		return assessment, files.Summary, nil
	}
	assessment.Complete = true
	assessment.Executions = inventory.executions
	assessment.Reason = "summary covers the expanded command snapshot and its recorded origins and attempts"
	if len(assessment.Executions) > 0 {
		assessment.Reason += "; execution termination is unknown and requires --assume-stopped"
	} else {
		assessment.Reason += "; inventory contains only jobs established never launched"
	}
	return assessment, files.Summary, nil
}

func (inventory *repairInventory) validate(runID string, files state.RunSnapshot) error {
	if files.CommandsErr != nil && !errors.Is(files.CommandsErr, os.ErrNotExist) {
		return fmt.Errorf("read repair commands: %w", files.CommandsErr)
	}
	if files.CarriedErr != nil && !errors.Is(files.CarriedErr, os.ErrNotExist) {
		return fmt.Errorf("read carried results: %w", files.CarriedErr)
	}
	summary := files.Summary
	if summary.RunID != runID {
		return incompleteRepair("summary names a different run")
	}
	start, err := time.Parse(time.RFC3339Nano, summary.StartedAt)
	if err != nil {
		return incompleteRepair("summary start time is invalid")
	}
	finish, err := time.Parse(time.RFC3339Nano, summary.FinishedAt)
	if err != nil || finish.Before(start) {
		return incompleteRepair("summary finish time is invalid")
	}
	if summary.Status != model.RunStatus(summary.ExitCode) && !(summary.Status == model.StatusCancelled && summary.ExitCode != 0) {
		return incompleteRepair("summary status and exit code disagree")
	}
	if errors.Is(files.CommandsErr, os.ErrNotExist) {
		return incompleteRepair("command snapshot is missing")
	}
	if len(files.Commands.Commands) == 0 {
		return incompleteRepair("command snapshot has no jobs")
	}
	for _, command := range files.Commands.Commands {
		if !state.IsValidPathElement(command.ID) || len(command.Command) == 0 {
			return incompleteRepair("command snapshot contains an invalid job")
		}
		if command.Array != nil {
			if err := model.ValidateArraySpec(command.Array); err != nil {
				return incompleteRepair("command snapshot contains an invalid array")
			}
		}
	}
	if err := model.ValidateQueueDependencies(files.Commands.Commands); err != nil {
		return incompleteRepair("command snapshot has invalid dependencies: " + err.Error())
	}
	jobs := model.QueueToJobs(files.Commands.Commands)
	expanded := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		if !state.IsValidPathElement(job.ID) || expanded[job.ID] {
			return incompleteRepair("expanded command snapshot duplicates or has an invalid job ID")
		}
		expanded[job.ID] = true
	}
	results := make(map[string]model.JobResult, len(summary.Results))
	failed := false
	for _, result := range summary.Results {
		if _, duplicate := results[result.ID]; duplicate {
			return incompleteRepair("summary duplicates job " + result.ID)
		}
		results[result.ID] = result
		failed = failed || result.ExitCode != 0
	}
	if (summary.ExitCode != 0) != failed {
		return incompleteRepair("summary exit code disagrees with its job results")
	}
	if len(results) != len(jobs) {
		return incompleteRepair("summary does not cover every expanded job exactly once")
	}
	carried := make(map[string]model.JobResult)
	if files.CarriedErr == nil {
		if files.Carried.RunID != runID {
			return incompleteRepair("carried record names a different run")
		}
		for _, result := range files.Carried.Results {
			if _, duplicate := carried[result.ID]; duplicate {
				return incompleteRepair("carried record duplicates job " + result.ID)
			}
			carried[result.ID] = result
		}
	}
	marks := model.QueueMarkedStatusByJobID(files.Commands)
	for _, job := range jobs {
		result, ok := results[job.ID]
		if !ok {
			return incompleteRepair("summary is missing job " + job.ID)
		}
		if len(result.Command) > 0 && !slices.Equal(result.Command, job.Command) {
			return incompleteRepair("summary command does not match job " + job.ID)
		}
		origin := repairJobOrigin(files.Commands, job)
		if saved, ok := carried[job.ID]; ok {
			if origin == nil || saved.AttemptID != result.AttemptID || saved.ExitCode != result.ExitCode || saved.Accepted != result.Accepted {
				return incompleteRepair("carried result does not match summary/origin for job " + job.ID)
			}
			delete(carried, job.ID)
		}
		marked := marks[job.ID]
		if err := inventory.job(runID, job, result, origin, marked, true, make(map[string]bool)); err != nil {
			return err
		}
	}
	if len(carried) != 0 {
		return incompleteRepair("carried record contains jobs outside the command snapshot")
	}
	// Unknown job directories must not disappear behind an apparently complete
	// summary. Config snapshots are not executions.
	entries, err := os.ReadDir(filepath.Join(inventory.paths.RunsDir, runID))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if (!entry.IsDir() && entry.Type()&os.ModeSymlink == 0) || expanded[entry.Name()] {
			continue
		}
		if entry.Name() == "configs" && entry.IsDir() {
			context, err := state.LoadContext(inventory.store, filepath.Join(inventory.paths.RunsDir, runID))
			if err != nil {
				return fmt.Errorf("read config snapshot inventory: %w", err)
			}
			children, err := os.ReadDir(filepath.Join(inventory.paths.RunsDir, runID, "configs"))
			if err != nil {
				return err
			}
			if len(children) != len(context.ConfigSnapshotFiles) || len(children) == 0 {
				return incompleteRepair("unrecognized config snapshot directory")
			}
			for _, child := range children {
				if !child.Type().IsRegular() || !slices.Contains(context.ConfigSnapshotFiles, child.Name()) {
					return incompleteRepair("unexpected execution or file in config snapshots")
				}
			}
			continue
		}
		return incompleteRepair("unexpected directory outside the execution inventory: " + entry.Name())
	}
	return nil
}

func (inventory *repairInventory) job(runID string, job model.JobSpec, result model.JobResult, origin *model.JobOrigin, marked string, latest bool, visiting map[string]bool) error {
	key := runID + "/" + job.ID
	if visiting[key] {
		return incompleteRepair("carried origin cycle at " + key)
	}
	visiting[key] = true
	defer delete(visiting, key)
	runDir := filepath.Join(inventory.paths.RunsDir, runID)
	jobDir, err := state.SafeJoin(runDir, job.ID)
	if err != nil {
		return incompleteRepair(err.Error())
	}
	entries, err := os.ReadDir(jobDir)
	absent := errors.Is(err, os.ErrNotExist)
	if err != nil && !absent {
		return fmt.Errorf("read execution inventory %s: %w", key, err)
	}
	undispatched := jobstatus.ReadAttempt(inventory.store, jobDir).Undispatched
	if origin != nil {
		if !undispatched {
			return incompleteRepair("carried job has local execution records: " + key)
		}
		return inventory.origin(job, result, *origin, marked, visiting)
	}
	if result.Accepted {
		return incompleteRepair("executed job cannot have an accepted carried result: " + key)
	}
	if undispatched && (strings.HasPrefix(result.Error, "blocked") || model.IsCancelledError(result.Error)) && result.ExitCode != 0 {
		if result.AttemptID != "" && !validRepairAttempt(result.AttemptID, runID, job.ID) {
			return incompleteRepair("invalid never-launched attempt association for " + key)
		}
		return nil
	}
	var attempts []string
	for _, entry := range entries {
		if entry.Name() != "attempts" {
			continue
		}
		if !entry.IsDir() {
			return incompleteRepair("attempts is not a directory for " + key)
		}
		children, err := os.ReadDir(filepath.Join(jobDir, "attempts"))
		if err != nil {
			return fmt.Errorf("read attempts for %s: %w", key, err)
		}
		for _, child := range children {
			if !child.IsDir() || !validRepairAttempt(child.Name(), runID, job.ID) {
				return incompleteRepair("invalid attempt directory for " + key)
			}
			attempts = append(attempts, child.Name())
		}
	}
	if result.AttemptID != "" {
		if !slices.Contains(attempts, result.AttemptID) {
			return incompleteRepair("summary attempt is missing or belongs to another job/run: " + key)
		}
		for _, attemptID := range attempts {
			payload, _ := state.DecodeAttemptIDForRun(attemptID, runID)
			selected, _ := state.DecodeAttemptIDForRun(result.AttemptID, runID)
			if latest && payload.Number > selected.Number {
				return incompleteRepair("summary omits a newer attempt for " + key)
			}
		}
	} else if len(attempts) > 0 {
		return incompleteRepair("summary does not identify the final attempt for " + key)
	}
	legacy := len(attempts) == 0
	for _, entry := range entries {
		if entry.Name() != "attempts" && entry.Name() != "cancelled" {
			legacy = true
		}
	}
	if legacy {
		if err := inventory.execution(runID, job, "", jobDir, result, result.AttemptID == ""); err != nil {
			return err
		}
	}
	for _, attemptID := range attempts {
		if err := inventory.execution(runID, job, attemptID, filepath.Join(jobDir, "attempts", attemptID), result, attemptID == result.AttemptID); err != nil {
			return err
		}
	}
	return nil
}

func validRepairAttempt(id, runID, jobID string) bool {
	payload, err := state.DecodeAttemptIDForRun(id, runID)
	return err == nil && payload.RunID == runID && payload.JobID == jobID && id == "att_"+runID+"-"+jobID+"-"+strconv.Itoa(payload.Number)
}

func (inventory *repairInventory) execution(runID string, job model.JobSpec, attemptID, dir string, result model.JobResult, final bool) error {
	key := runID + "/" + job.ID + "/" + attemptID
	if inventory.seen[key] {
		return nil
	}
	inventory.seen[key] = true
	var command model.JobSpec
	err := inventory.store.ReadJSON(filepath.Join(dir, "command.json"), &command)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read execution command %s: %w", key, err)
	}
	if err == nil && (command.ID != job.ID || command.AttemptID != attemptID || !slices.Equal(command.Command, job.Command)) {
		return incompleteRepair("attempt command does not match execution " + key)
	}
	attempt := jobstatus.ReadAttempt(inventory.store, dir)
	if final && attempt.Finished() && attempt.ExitCode != result.ExitCode && !result.Accepted {
		return incompleteRepair("recorded outcome conflicts with summary for " + key)
	}
	backend := command.Executor
	if backend == "" {
		backend = job.Executor
	}
	if backend == "" {
		backend = "unknown"
	}
	inventory.executions = append(inventory.executions, model.RecoveryExecution{
		RunID: runID, JobID: job.ID, AttemptID: attemptID, Backend: backend,
		State: "unknown", Reason: "persisted outcome does not establish wrapper, managed descendants, or allocation termination",
	})
	return nil
}

func (inventory *repairInventory) origin(job model.JobSpec, result model.JobResult, origin model.JobOrigin, marked string, visiting map[string]bool) error {
	if !state.IsValidPathElement(origin.RunID) || !state.IsValidPathElement(origin.JobID) || (origin.AttemptID != "" && origin.AttemptID != result.AttemptID) {
		return incompleteRepair("invalid carried origin for " + job.ID)
	}
	runDir := filepath.Join(inventory.paths.RunsDir, origin.RunID)
	files, err := state.LoadRunSnapshot(runDir)
	if err != nil {
		return err
	}
	if files.CommandsErr != nil {
		if errors.Is(files.CommandsErr, os.ErrNotExist) {
			return incompleteRepair("origin command snapshot is missing for " + job.ID)
		}
		return fmt.Errorf("read origin commands: %w", files.CommandsErr)
	}
	for _, fileErr := range []error{files.SummaryErr, files.CarriedErr} {
		if fileErr != nil && !errors.Is(fileErr, os.ErrNotExist) {
			return fmt.Errorf("read origin results: %w", fileErr)
		}
	}
	var spec *model.JobSpec
	for _, candidate := range model.QueueToJobs(files.Commands.Commands) {
		if candidate.ID == origin.JobID {
			if spec != nil {
				return incompleteRepair("origin command snapshot duplicates " + origin.JobID)
			}
			spec = &candidate
		}
	}
	if spec == nil {
		return incompleteRepair("carried origin job is missing for " + job.ID)
	}
	if !slices.Equal(spec.Command, job.Command) {
		return incompleteRepair("carried command disagrees with origin for " + job.ID)
	}
	for _, record := range []struct {
		summary model.RunSummary
		err     error
	}{{files.Summary, files.SummaryErr}, {files.Carried, files.CarriedErr}} {
		if record.err != nil {
			continue
		}
		if record.summary.RunID != origin.RunID {
			return incompleteRepair("origin result record names a different run for " + job.ID)
		}
		ids := make(map[string]bool)
		for _, candidate := range record.summary.Results {
			if ids[candidate.ID] {
				return incompleteRepair("origin result record duplicates job " + candidate.ID)
			}
			ids[candidate.ID] = true
		}
	}
	recorded, found := jobstatus.RecordedResults(files)[origin.JobID]
	attemptID := origin.AttemptID
	if attemptID == "" {
		attemptID = result.AttemptID
	}
	if !found && attemptID == "" {
		recorded, found = jobstatus.ReadAttempt(inventory.store, filepath.Join(runDir, origin.JobID)).Result(*spec)
	}
	if attemptID != "" && (!found || recorded.AttemptID != attemptID) {
		if !validRepairAttempt(attemptID, origin.RunID, origin.JobID) {
			return incompleteRepair("invalid carried attempt for " + job.ID)
		}
		dir := filepath.Join(runDir, origin.JobID, "attempts", attemptID)
		attempt := jobstatus.ReadAttempt(inventory.store, dir)
		recorded, found = attempt.Result(*spec)
		recorded.AttemptID = attemptID
	}
	if !found || recorded.AttemptID != result.AttemptID {
		return incompleteRepair("carried origin has no recorded result for " + job.ID)
	}
	expected, finished, err := model.MarkResult(job.ID, recorded, true, marked)
	if err != nil || !finished || expected.ExitCode != result.ExitCode || expected.Accepted != result.Accepted || model.ResultStatus(expected, true) != model.ResultStatus(result, true) {
		return incompleteRepair("carried outcome disagrees with origin and status mark for " + job.ID)
	}
	sourceMarked := model.QueueMarkedStatusByJobID(files.Commands)[origin.JobID]
	return inventory.job(origin.RunID, *spec, recorded, repairJobOrigin(files.Commands, *spec), sourceMarked, origin.AttemptID == "", visiting)
}

func repairJobOrigin(queue model.Queue, job model.JobSpec) *model.JobOrigin {
	if job.ArrayTaskID != nil {
		for _, command := range queue.Commands {
			if command.ID == job.ArrayGroup {
				return model.TaskOrigin(command, *job.ArrayTaskID)
			}
		}
	}
	return queue.OriginOf(job.ID)
}

func finalizeRepair(paths state.ProjectPaths, runDir string, meta model.Meta, summary model.RunSummary, store state.Store) error {
	files, err := state.ReadQueueFile(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return err
	}
	collision := false
	for _, job := range model.QueueToJobs(files.Commands) {
		collision = collision || job.ID == state.RunClientStatusFileName
	}
	if !collision {
		client, err := state.LoadRunClientStatus(store, runDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read recovery client status: %w", err)
		}
		if err == nil && client.State != model.RunClientCompleted {
			client.State = model.RunClientCompleted
			client.UpdatedAt = summary.FinishedAt
			if err := state.WriteRunClientStatus(store, runDir, client); err != nil {
				return fmt.Errorf("complete recovery client status: %w", err)
			}
		}
	}
	finishedAt, err := time.Parse(time.RFC3339Nano, summary.FinishedAt)
	if err != nil {
		return err
	}
	meta, err = state.FinalizeRun(meta, summary.RunID, summary.ExitCode, finishedAt)
	if err != nil {
		return err
	}
	return store.WriteJSON(paths.MetaFile, meta)
}
