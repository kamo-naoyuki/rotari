package project

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
)

type CoordinatorState string

const (
	CoordinatorLive    CoordinatorState = "live"
	CoordinatorStopped CoordinatorState = "stopped"
	CoordinatorUnknown CoordinatorState = "unknown"
)

// CoordinatorAssessment describes process ownership, not job liveness or
// permission to unlock. A management completion marker never overrides a probe.
type CoordinatorAssessment struct {
	State      CoordinatorState      `json:"state"`
	Reason     string                `json:"reason"`
	ObservedAt string                `json:"observed_at"`
	Identity   *model.RunCoordinator `json:"identity,omitempty"`
}

// AssessCoordinator reads the durable run context, independently of running.lock.
// Missing legacy identity is unknown; malformed or unreadable records are errors.
func AssessCoordinator(store state.Store, runDir string) (CoordinatorAssessment, error) {
	result := CoordinatorAssessment{State: CoordinatorUnknown, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	context, err := state.LoadContext(store, runDir)
	if errors.Is(err, os.ErrNotExist) {
		result.Reason = "run context is missing"
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("load coordinator identity: %w", err)
	}
	result.Identity = context.Coordinator
	host, err := os.Hostname()
	if err != nil {
		return result, fmt.Errorf("identify inspection host: %w", err)
	}
	identity := context.Coordinator
	result.State, result.Reason = coordinatorLocation(identity, host, state.PIDNamespace())
	if result.Reason != "" {
		return result, nil
	}
	// Probe only identities in this host and namespace. Never signal an
	// unvalidated remote PID, even with signal zero.
	probeErr := syscall.Kill(identity.PID, 0)
	result.State, result.Reason = coordinatorProcess(identity.ProcessStart, state.ProcessStart(identity.PID), probeErr)
	return result, nil
}

func coordinatorLocation(identity *model.RunCoordinator, host, namespace string) (CoordinatorState, string) {
	switch {
	case identity == nil:
		return CoordinatorUnknown, "coordinator identity was not recorded"
	case identity.Host == "" || identity.PID <= 0:
		return CoordinatorUnknown, "coordinator identity is incomplete"
	case identity.Host != host:
		return CoordinatorUnknown, "coordinator belongs to another host"
	case identity.PIDNamespace == "" || namespace == "":
		return CoordinatorUnknown, "coordinator PID namespace cannot be verified"
	case identity.PIDNamespace != namespace:
		return CoordinatorUnknown, "coordinator belongs to another PID namespace"
	default:
		return CoordinatorUnknown, ""
	}
}

func coordinatorProcess(recordedStart, currentStart string, probeErr error) (CoordinatorState, string) {
	switch {
	case errors.Is(probeErr, syscall.ESRCH):
		return CoordinatorStopped, "coordinator process no longer exists"
	case probeErr != nil:
		return CoordinatorUnknown, fmt.Sprintf("cannot inspect coordinator process: %v", probeErr)
	case recordedStart == "" || currentStart == "":
		return CoordinatorUnknown, "coordinator process start time cannot be verified; process may still be alive"
	case recordedStart != currentStart:
		return CoordinatorStopped, "PID belongs to a different process"
	default:
		return CoordinatorLive, "coordinator process identity matches"
	}
}
