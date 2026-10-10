package queueops

import (
	"fmt"
	"os"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// AddNote adds a note to the run runID of the project at paths, or, with an
// attemptID, to that job attempt of the run. The run may be active or
// finished: notes are appended and change none of its results.
func AddNote(paths state.ProjectPaths, runID, attemptID, text string, now time.Time) (model.RunNote, error) {
	text, err := model.NoteText(text)
	if err != nil {
		return model.RunNote{}, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return model.RunNote{}, err
	}
	if info, err := os.Stat(runDir); err != nil || !info.IsDir() {
		return model.RunNote{}, fmt.Errorf("run %q not found in project %q", runID, paths.ProjectName)
	}
	note := model.RunNote{At: now.UTC().Format(time.RFC3339), Text: text}
	if attemptID != "" {
		payload, err := state.DecodeAttemptID(attemptID)
		if err != nil {
			return model.RunNote{}, err
		}
		if payload.RunID != runID {
			return model.RunNote{}, fmt.Errorf("attempt %q belongs to run %q, not %q", attemptID, payload.RunID, runID)
		}
		attemptDir, err := state.SpecificAttemptJobDir(runDir, payload.JobID, attemptID)
		if err != nil {
			return model.RunNote{}, err
		}
		if info, err := os.Stat(attemptDir); err != nil || !info.IsDir() {
			return model.RunNote{}, fmt.Errorf("attempt %q not found in run %q", attemptID, runID)
		}
		note.JobID, note.AttemptID = payload.JobID, attemptID
	}
	if err := state.AppendRunNote(runDir, note); err != nil {
		return model.RunNote{}, fmt.Errorf("failed to save note: %w", err)
	}
	return note, nil
}
