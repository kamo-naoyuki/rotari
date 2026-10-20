package project

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

const (
	recoveryEvidenceFileLimit    = 16 << 20
	recoveryEvidenceByteLimit    = 64 << 20
	recoveryEvidenceEntryLimit   = 100000
	recoveryObservationFreshness = 30 * time.Second
)

var recoveryEvidenceFiles = map[string]bool{
	"commands.json": true, "summary.json": true, "carried.json": true,
	"context.json": true, "job.json": true, "command.json": true,
	"status": true, "status.json": true, "scheduler_status.json": true,
	"pid": true, "cancelled": true, "submitted_at": true, "finished_at": true,
	state.RunClientStatusFileName: true,
}

// recoveryRevision binds an unlock preview to the queue/meta revision and
// the run files that define execution identity and recorded outcomes.
func recoveryRevision(paths state.ProjectPaths, runID string) (string, error) {
	revision, err := Revision(paths)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write([]byte(revision))
	total := 0
	if err := hashRecoveryFile(hash, paths.LockFile, &total); err != nil {
		return "", err
	}
	runIDs, err := recoveryRunIDs(paths, runID)
	if err != nil {
		return "", err
	}
	entries := 0
	for _, id := range runIDs {
		runDir, err := state.SafeJoin(paths.RunsDir, id)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write([]byte("run:" + id + "\n"))
		if _, err := os.Lstat(runDir); errors.Is(err, os.ErrNotExist) {
			_, _ = hash.Write([]byte("missing-dir:" + filepath.ToSlash(runDir) + "\n"))
			continue
		} else if err != nil {
			return "", fmt.Errorf("fingerprint execution evidence: %w", err)
		}
		err = filepath.WalkDir(runDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			entries++
			if entries > recoveryEvidenceEntryLimit {
				return fmt.Errorf("recovery evidence exceeds the %d-entry limit", recoveryEvidenceEntryLimit)
			}
			rel, err := filepath.Rel(runDir, path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				_, _ = hash.Write([]byte("dir:" + filepath.ToSlash(rel) + "\n"))
				return nil
			}
			if !recoveryEvidenceFiles[entry.Name()] {
				return nil
			}
			return hashRecoveryFile(hash, path, &total)
		})
		if err != nil {
			return "", fmt.Errorf("fingerprint execution evidence: %w", err)
		}
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

func recoveryRunIDs(paths state.ProjectPaths, runID string) ([]string, error) {
	runIDs := []string{runID}
	seen := map[string]bool{runID: true}
	for i := 0; i < len(runIDs); i++ {
		runDir, err := state.SafeJoin(paths.RunsDir, runIDs[i])
		if err != nil {
			return nil, err
		}
		files, err := state.LoadRunSnapshot(runDir)
		if err != nil {
			return nil, err
		}
		if files.CommandsErr != nil {
			continue
		}
		for _, job := range model.QueueToJobs(files.Commands.Commands) {
			origin := repairJobOrigin(files.Commands, job)
			if origin == nil {
				continue
			}
			if !state.IsValidPathElement(origin.RunID) {
				return nil, fmt.Errorf("invalid carried origin run ID %q", origin.RunID)
			}
			if !seen[origin.RunID] {
				seen[origin.RunID] = true
				runIDs = append(runIDs, origin.RunID)
			}
		}
	}
	sort.Strings(runIDs)
	return runIDs, nil
}

func hashRecoveryFile(hash interface{ Write([]byte) (int, error) }, path string, total *int) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		_, _ = hash.Write([]byte("missing:" + filepath.ToSlash(path) + "\n"))
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > recoveryEvidenceFileLimit {
		return fmt.Errorf("%s exceeds the %d-byte recovery evidence limit", path, recoveryEvidenceFileLimit)
	}
	*total += len(data)
	if *total > recoveryEvidenceByteLimit {
		return fmt.Errorf("recovery evidence exceeds the %d-byte limit", recoveryEvidenceByteLimit)
	}
	_, _ = hash.Write([]byte("file:" + filepath.ToSlash(path) + "\n"))
	_, _ = hash.Write([]byte(fmt.Sprintf("%d\n", len(data))))
	_, _ = hash.Write(data)
	return nil
}

func expireStaleExecutionEvidence(executions []model.RecoveryExecution, now time.Time) {
	const staleReason = "execution observation is missing or older than 30 seconds and cannot authorize recovery"
	for i := range executions {
		observed, err := time.Parse(time.RFC3339Nano, executions[i].ObservedAt)
		if err == nil && now.Sub(observed) <= recoveryObservationFreshness && !observed.After(now) {
			continue
		}
		if executions[i].State == "active" {
			executions[i].PossiblyActive = true
		}
		executions[i].State = "unknown"
		executions[i].Conflict = executions[i].Conflict || executions[i].RecordedTerminal
		if !strings.Contains(executions[i].Reason, staleReason) {
			if executions[i].Reason != "" {
				executions[i].Reason += "; "
			}
			executions[i].Reason += staleReason
		}
	}
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
