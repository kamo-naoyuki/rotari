package project

import (
	"path/filepath"

	"github.com/rotari-dev/rotari/internal/state"
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
// the run's summary. Like Inspect, it writes nothing but the removal of a
// stale lock. The lock is read before the summary, and a run writes its
// summary before it releases the lock, so a run that finishes in between is
// reported as finished.
func RunPhaseOf(paths state.ProjectPaths, runID string) (RunPhase, error) {
	inspection, err := Inspect(paths)
	if err != nil {
		return "", err
	}
	if phase := PhaseFrom(inspection, runID, false); phase == RunPhaseRunning || phase == RunPhaseInterrupted {
		return phase, nil
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", err
	}
	_, err = state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	return PhaseFrom(inspection, runID, err == nil), nil
}

// PhaseFrom is RunPhaseOf from facts a caller already read: the project's
// inspection, read before the run's summary as RunPhaseOf reads them, and
// whether the run has a valid summary. A reader of many runs inspects the
// project once.
func PhaseFrom(inspection Inspection, runID string, hasSummary bool) RunPhase {
	if inspection.RunID == runID {
		switch inspection.State {
		case Running:
			return RunPhaseRunning
		case Interrupted:
			return RunPhaseInterrupted
		}
	}
	if hasSummary {
		return RunPhaseFinished
	}
	return RunPhaseEnded
}
