package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
)

// FinalizeRun returns meta marked finished with runID's exit code.
func FinalizeRun(meta model.Meta, runID string, exitCode int, now time.Time) (model.Meta, error) {
	if runID == "" {
		return model.Meta{}, fmt.Errorf("run ID must not be empty")
	}
	meta.Phase = "finished"
	meta.LastRunID = runID
	meta.LastRunExitCode = exitCode
	meta.UpdatedAt = now.UTC().Format(time.RFC3339)
	return meta, nil
}

// LoadRunOrigin returns jobID's origin from the run's commands.json snapshot,
// or nil when the job has none or the snapshot cannot be read; see
// model.Queue.OriginOf.
func LoadRunOrigin(runDir, jobID string) *model.JobOrigin {
	queue, err := LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil
	}
	return queue.OriginOf(jobID)
}

// CheckRunVersions returns ErrNewerStateVersion when the run's commands.json
// or summary.json was written by a newer rotari, and nil otherwise, leaving a
// missing or unreadable file to the caller. Readers that treat those files
// as optional call it first, so a newer file is refused instead of being read
// as absent.
func CheckRunVersions(runDir string) error {
	_, err := LoadRunSnapshot(runDir)
	return err
}

// RunSnapshot is what a run directory records about the whole run:
// commands.json, summary.json, and carried.json, each read once. An error
// field is os.ErrNotExist for a missing file.
type RunSnapshot struct {
	Commands    model.Queue
	CommandsErr error
	Summary     model.RunSummary
	SummaryErr  error
	Carried     model.RunSummary
	CarriedErr  error
}

// LoadRunSnapshot reads runDir's run files. It fails, with
// ErrNewerStateVersion, only when one was written by a newer rotari; any
// other error is kept in the snapshot for the caller.
func LoadRunSnapshot(runDir string) (RunSnapshot, error) {
	var snapshot RunSnapshot
	snapshot.Commands, snapshot.CommandsErr = ReadQueueFile(filepath.Join(runDir, "commands.json"))
	snapshot.Summary, snapshot.SummaryErr = LoadRunSummary(filepath.Join(runDir, "summary.json"))
	snapshot.Carried, snapshot.CarriedErr = LoadRunSummary(filepath.Join(runDir, CarriedResultsFileName))
	for _, err := range []error{snapshot.CommandsErr, snapshot.SummaryErr, snapshot.CarriedErr} {
		if errors.Is(err, ErrNewerStateVersion) {
			return RunSnapshot{}, err
		}
	}
	return snapshot, nil
}

// Queue is the run's command snapshot as LoadQueue reads it: empty when the
// file is missing.
func (snapshot RunSnapshot) Queue() (model.Queue, error) {
	if errors.Is(snapshot.CommandsErr, os.ErrNotExist) {
		return model.Queue{}, nil
	}
	return snapshot.Commands, snapshot.CommandsErr
}

// RecordedSummary is the run's summary, or nil when it has no readable one.
func (snapshot RunSnapshot) RecordedSummary() *model.RunSummary {
	if snapshot.SummaryErr != nil {
		return nil
	}
	return &snapshot.Summary
}

// CarriedResultsFileName is the run file in which a run records, before it
// dispatches any job, the results it carries forward from earlier runs
// instead of executing. It holds a model.RunSummary with only RunID and
// Results, and is read through LoadRunSummary until the run writes
// summary.json; see jobstatus.RecordedResults.
const CarriedResultsFileName = "carried.json"

func LoadRunSummary(path string) (model.RunSummary, error) {
	var summary model.RunSummary
	if err := NewStore(0o700, 0o600).ReadJSON(path, &summary); err != nil {
		return model.RunSummary{}, err
	}
	if err := checkStateVersion(path, summary.StateVersion); err != nil {
		return model.RunSummary{}, err
	}
	return summary, nil
}

// LoadRunSummaryHead is LoadRunSummary without the per-job results, which
// it leaves nil: a listing of many runs needs only their times, names,
// status, and exit code, and a run of many jobs has a large summary.
func LoadRunSummaryHead(path string) (model.RunSummary, error) {
	var head struct {
		StateVersion int    `json:"state_version"`
		RunID        string `json:"run_id"`
		RunName      string `json:"run_name"`
		Status       string `json:"status"`
		StartedAt    string `json:"started_at"`
		FinishedAt   string `json:"finished_at"`
		ExitCode     int    `json:"exit_code"`
	}
	if err := NewStore(0o700, 0o600).ReadJSON(path, &head); err != nil {
		return model.RunSummary{}, err
	}
	if err := checkStateVersion(path, head.StateVersion); err != nil {
		return model.RunSummary{}, err
	}
	return model.RunSummary{StateVersion: head.StateVersion, RunID: head.RunID, RunName: head.RunName, Status: head.Status,
		StartedAt: head.StartedAt, FinishedAt: head.FinishedAt, ExitCode: head.ExitCode}, nil
}

// RunSourcesFileName is the run file in which a run records, before it
// dispatches any job, the version-control revisions its executed jobs run
// from. A run started before rotari recorded sources has none.
const RunSourcesFileName = "sources.json"

// SaveRunSources writes a run's sources.
func SaveRunSources(runDir string, sources model.RunSources) error {
	return WriteJSON(filepath.Join(runDir, RunSourcesFileName), sources)
}

// LoadRunSources reads a run's sources, and false when the run recorded
// none.
func LoadRunSources(runDir string) (model.RunSources, bool, error) {
	var sources model.RunSources
	path := filepath.Join(runDir, RunSourcesFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return model.RunSources{}, false, nil
	}
	if err != nil {
		return model.RunSources{}, false, err
	}
	if err := json.Unmarshal(data, &sources); err != nil {
		return model.RunSources{}, false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return sources, true, nil
}

// AttemptSource returns the recorded revision of the repository the attempt
// in attemptDir ran from: its working directory, from its command.json,
// matched against its run's sources. It returns false when either is not
// recorded.
func AttemptSource(runDir, attemptDir string) (model.SourceRevision, bool) {
	var spec model.JobSpec
	data, err := os.ReadFile(filepath.Join(attemptDir, "command.json"))
	if err != nil || json.Unmarshal(data, &spec) != nil || spec.WorkingDirectory == "" {
		return model.SourceRevision{}, false
	}
	sources, ok, err := LoadRunSources(runDir)
	if err != nil || !ok {
		return model.SourceRevision{}, false
	}
	return sources.SourceFor(spec.WorkingDirectory)
}

// RunNotesFileName is the run file holding a run's notes, one JSON
// model.RunNote per line, in the order they were added. It is the one run
// file that changes after a run finishes, and only by appending.
const RunNotesFileName = "notes.jsonl"

// AppendRunNote adds a note to the run in runDir. Each note is written with
// one append, so notes added at the same time do not interleave.
func AppendRunNote(runDir string, note model.RunNote) error {
	data, err := json.Marshal(note)
	if err != nil {
		return err
	}
	// codeql[go/path-injection]: runDir is a validated run directory and the file name is fixed.
	file, err := os.OpenFile(filepath.Join(runDir, RunNotesFileName), os.O_WRONLY|os.O_APPEND|os.O_CREATE, FileMode())
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// LoadRunNotes reads a run's notes in the order they were added; a run
// without notes has none.
func LoadRunNotes(runDir string) ([]model.RunNote, error) {
	data, err := os.ReadFile(filepath.Join(runDir, RunNotesFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	notes := make([]model.RunNote, 0)
	for number, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var note model.RunNote
		if err := json.Unmarshal([]byte(line), &note); err != nil {
			return nil, fmt.Errorf("failed to read %s line %d: %w", RunNotesFileName, number+1, err)
		}
		notes = append(notes, note)
	}
	return notes, nil
}
