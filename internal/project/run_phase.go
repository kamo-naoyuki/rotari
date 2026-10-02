package project

import (
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// RunPhase is where one run is in its life, as a reader sees it.
type RunPhase string

const (
	// RunPhaseRunning means the run holds the project's run lock, including
	// before it has written its job snapshot.
	RunPhaseRunning RunPhase = "running"
	// RunPhaseInterrupted means metadata still records the run as active but
	// no live coordinator holds the lock.
	RunPhaseInterrupted RunPhase = "interrupted"
	// RunPhaseFinished means the run is not active and has a valid summary.
	RunPhaseFinished RunPhase = "finished"
	// RunPhaseEnded means the run is not active and has no valid summary,
	// for example because its supervisor exited early.
	RunPhaseEnded RunPhase = "ended"
)

// RunPhaseOf reports runID's phase from the project's lock and metadata and
// the run's summary, without writing. The lock is read before the summary,
// and a run writes its summary before it releases the lock, so a run that
// finishes in between is reported as finished.
func RunPhaseOf(paths state.ProjectPaths, runID string) (RunPhase, error) {
	inspection, err := Inspect(paths, false)
	if err != nil {
		return "", err
	}
	if inspection.RunID == runID {
		switch inspection.State {
		case Running:
			return RunPhaseRunning, nil
		case Interrupted:
			return RunPhaseInterrupted, nil
		}
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", err
	}
	if _, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		return RunPhaseFinished, nil
	}
	return RunPhaseEnded, nil
}
