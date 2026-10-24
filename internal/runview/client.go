package runview

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/rotari-dev/rotari/internal/attachment"
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/project"
	"github.com/rotari-dev/rotari/internal/state"
)

// ClientStatus reads the initiating client's recorded state and rejects stale
// live-attachment claims unless a local supervisor is confirmed alive.
func ClientStatus(paths state.ProjectPaths, runID string) (model.RunClientStatus, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return model.RunClientStatus{}, err
	}
	phase, err := project.RunPhaseOf(paths, runID)
	if err != nil {
		return model.RunClientStatus{}, err
	}
	return clientStatusIn(paths, runDir, runID, phase)
}

// ClientStatusIn is ClientStatus for a run whose phase a caller already
// read (project.PhaseFrom).
func ClientStatusIn(paths state.ProjectPaths, runID string, phase project.RunPhase) (model.RunClientStatus, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return model.RunClientStatus{}, err
	}
	return clientStatusIn(paths, runDir, runID, phase)
}

func clientStatusIn(paths state.ProjectPaths, runDir, runID string, phase project.RunPhase) (model.RunClientStatus, error) {
	status, statusErr := state.LoadRunClientStatus(state.NewStore(state.DirectoryMode(), state.FileMode()), runDir)
	if statusErr != nil && !errors.Is(statusErr, os.ErrNotExist) {
		status = model.RunClientStatus{State: "unknown"}
	}
	switch phase {
	case project.RunPhaseRunning:
		return activeClientStatus(paths, runID, status, statusErr)
	case project.RunPhaseInterrupted:
		status.State = "unknown"
		return status, nil
	default:
		return settledClientStatus(status, statusErr), nil
	}
}

func activeClientStatus(paths state.ProjectPaths, runID string, status model.RunClientStatus, statusErr error) (model.RunClientStatus, error) {
	if attachment.Enabled(paths, runID) {
		attached, err := attachment.Attached(paths, runID)
		if err != nil {
			return model.RunClientStatus{}, err
		}
		if attached {
			status.State = model.RunClientAttached
		} else if status.State == model.RunClientAttached {
			// The old record may outlive a killed client. Session scan errs on
			// uncertainty, so no live sessions means the old claim is stale.
			status.State = "unknown"
		}
		return status, nil
	}
	if attached, err := attachment.Attached(paths, runID); err != nil {
		return model.RunClientStatus{}, err
	} else if attached {
		status.State = model.RunClientAttached
		return status, nil
	}
	lockState, lock, err := project.InspectRunLock(paths)
	if err != nil {
		return model.RunClientStatus{}, err
	}
	if lock.RunID != runID || lockState != state.LockActive {
		status.State = "unknown"
		return status, nil
	}
	if status.State == model.RunClientAttached && !lock.ClientAttached {
		status.State = "unknown"
		return status, nil
	}
	if !errors.Is(statusErr, os.ErrNotExist) {
		return status, nil
	}
	if lock.ClientAttached {
		return model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientAttached}, nil
	}
	return model.RunClientStatus{State: "unknown"}, nil
}

func settledClientStatus(status model.RunClientStatus, statusErr error) model.RunClientStatus {
	if errors.Is(statusErr, os.ErrNotExist) || status.State != model.RunClientCompleted {
		status.State = "unknown"
	}
	return status
}

// ClientStatusLabel formats client attachment independently from run lifecycle.
func ClientStatusLabel(status model.RunClientStatus) string {
	switch status.State {
	case model.RunClientAttached:
		return "attached"
	case model.RunClientDetached:
		return detachedClientLabel(status)
	case model.RunClientCancelling:
		return "disconnecting/cancelling"
	case model.RunClientCompleted:
		return completedClientLabel(status)
	default:
		return unknownClientLabel(status)
	}
}

func detachedClientLabel(status model.RunClientStatus) string {
	switch status.Reason {
	case model.RunClientReasonAsync:
		return "detached (async)"
	case model.RunClientReasonCtrlD:
		return "detached (Ctrl-D)"
	case model.RunClientReasonEOF:
		return "detached (disconnect)"
	default:
		return "detached"
	}
}

func completedClientLabel(status model.RunClientStatus) string {
	if status.Mode == model.RunClientModeAsync {
		return "async (completed)"
	}
	switch status.Reason {
	case model.RunClientReasonCtrlD:
		return "sync (completed; Ctrl-D detached)"
	case model.RunClientReasonEOF:
		return "sync (completed; disconnected)"
	case model.RunClientReasonCancel:
		return "sync (completed; cancelled after disconnect)"
	case model.RunClientReasonCtrlC:
		return "sync (completed; Ctrl-C)"
	default:
		return "sync (completed)"
	}
}

func unknownClientLabel(status model.RunClientStatus) string {
	if status.Mode == model.RunClientModeAsync {
		return "async (unknown)"
	}
	switch status.Reason {
	case model.RunClientReasonCtrlD:
		return "unknown (last detached by Ctrl-D)"
	case model.RunClientReasonEOF:
		return "unknown (last detached by disconnect)"
	case model.RunClientReasonCancel, model.RunClientReasonCtrlC:
		return "unknown (last cancelling)"
	default:
		return "unknown"
	}
}

// RunLifecycleLabel names a run's persisted lifecycle for status views.
func RunLifecycleLabel(paths state.ProjectPaths, runID string) (string, error) {
	phase, err := project.RunPhaseOf(paths, runID)
	if err != nil {
		return "", err
	}
	var summary model.RunSummary
	if phase == project.RunPhaseFinished {
		runDir, err := state.SafeJoin(paths.RunsDir, runID)
		if err != nil {
			return "", err
		}
		if summary, err = state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err != nil {
			return "", err
		}
	}
	return LifecycleLabel(phase, summary), nil
}

// LifecycleLabel is RunLifecycleLabel from facts a caller already read: the
// run's phase and, for a finished run, its summary.
func LifecycleLabel(phase project.RunPhase, summary model.RunSummary) string {
	switch phase {
	case project.RunPhaseRunning:
		return "running"
	case project.RunPhaseInterrupted:
		return "interrupted"
	case project.RunPhaseFinished:
		if summary.ExitCode == 0 {
			return "finished"
		}
		if summary.Status == model.StatusCancelled {
			return model.StatusCancelled
		}
		return "failed"
	default:
		return "incomplete"
	}
}
