package project

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type ExecutionAssessment struct {
	Stopped    int                       `json:"stopped"`
	Active     int                       `json:"active"`
	Unknown    int                       `json:"unknown"`
	Executions []model.RecoveryExecution `json:"executions"`
}

// AssessExecutions reads all relevant attempts, not only attempts that already
// have a command file or a final result. It never changes run or outcome files.
func AssessExecutions(paths state.ProjectPaths, runID string) (ExecutionAssessment, error) {
	assessment := ExecutionAssessment{Executions: []model.RecoveryExecution{}}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return assessment, err
	}
	files, err := state.LoadRunSnapshot(runDir)
	if err != nil {
		return assessment, err
	}
	for _, fileErr := range []error{files.SummaryErr, files.CarriedErr} {
		if fileErr != nil && !errors.Is(fileErr, os.ErrNotExist) {
			return assessment, fmt.Errorf("read execution outcome inventory: %w", fileErr)
		}
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	inventory := repairInventory{paths: paths, store: store, seen: make(map[string]bool), observationOnly: true, recorded: make(map[string]bool)}
	addUnknown := func(id, reason string) {
		inventory.executions = append(inventory.executions, model.RecoveryExecution{RunID: runID, JobID: id, Backend: "unknown", State: "unknown", Source: "inventory", Reason: reason, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Scope: "execution inventory is incomplete; no termination conclusion"})
	}
	if files.CommandsErr != nil {
		if !errors.Is(files.CommandsErr, os.ErrNotExist) {
			return assessment, fmt.Errorf("read execution command snapshot: %w", files.CommandsErr)
		}
		addUnknown("", "command snapshot is missing; execution inventory cannot be established")
	} else {
		results := jobstatus.RecordedResults(files)
		marks := model.QueueMarkedStatusByJobID(files.Commands)
		jobs := model.QueueToJobs(files.Commands.Commands)
		if len(jobs) == 0 {
			addUnknown("", "command snapshot has no expanded execution units")
		}
		seen := make(map[string]bool)
		for _, job := range jobs {
			if seen[job.ID] {
				addUnknown(job.ID, "command snapshot duplicates an execution unit")
				continue
			}
			seen[job.ID] = true
			result, recorded := results[job.ID]
			if recorded {
				inventory.recorded[runID+"/"+job.ID] = true
			}
			origin := repairJobOrigin(files.Commands, job)
			if !recorded {
				attemptID, dir, err := state.LatestAttempt(runDir, job.ID)
				if err != nil {
					return assessment, err
				}
				attempt := jobstatus.ReadAttempt(store, dir)
				result, _ = attempt.Result(job)
				result.ID, result.AttemptID = job.ID, attemptID
			}
			err := inventory.job(runID, job, result, origin, marks[job.ID], true, make(map[string]bool))
			if err != nil {
				var incomplete incompleteRepair
				if !errors.As(err, &incomplete) {
					return assessment, err
				}
				addUnknown(job.ID, incomplete.Error())
			}
		}
		if err := inventory.validateDirectories(runID, seen); err != nil {
			var incomplete incompleteRepair
			if !errors.As(err, &incomplete) {
				return assessment, err
			}
			addUnknown("", incomplete.Error())
		}
	}
	var toProbe []model.RecoveryExecution
	for _, execution := range inventory.executions {
		if execution.Source == "inventory" {
			assessment.Executions = append(assessment.Executions, execution)
		} else {
			toProbe = append(toProbe, execution)
		}
	}
	assessment.Executions = append(assessment.Executions, probeRecoveryExecutions(store, paths, toProbe)...)
	for _, execution := range assessment.Executions {
		switch execution.State {
		case "stopped":
			assessment.Stopped++
		case "active":
			assessment.Active++
		default:
			assessment.Unknown++
		}
	}
	return assessment, nil
}
