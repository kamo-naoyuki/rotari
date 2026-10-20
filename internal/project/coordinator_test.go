package project

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCoordinatorProcessEvidence(t *testing.T) {
	for _, test := range []struct {
		name, recorded, current string
		probeErr                error
		want                    CoordinatorState
	}{
		{"matching process", "100", "100", nil, CoordinatorLive},
		{"reused PID", "100", "200", nil, CoordinatorStopped},
		{"missing recorded start", "", "100", nil, CoordinatorUnknown},
		{"unreadable current start", "100", "", nil, CoordinatorUnknown},
		{"process gone", "100", "", syscall.ESRCH, CoordinatorStopped},
		{"legacy process gone", "", "", syscall.ESRCH, CoordinatorStopped},
		{"permission denied", "100", "100", syscall.EPERM, CoordinatorUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, reason := coordinatorProcess(test.recorded, test.current, test.probeErr)
			if got != test.want || reason == "" {
				t.Fatalf("assessment = %s (%s), want %s with evidence", got, reason, test.want)
			}
		})
	}
}

func TestCoordinatorLocationEvidence(t *testing.T) {
	for _, test := range []struct {
		name     string
		identity *model.RunCoordinator
		want     string
	}{
		{"legacy", nil, "not recorded"},
		{"invalid PID", &model.RunCoordinator{Host: "here", PID: -1}, "incomplete"},
		{"missing host", &model.RunCoordinator{PID: 1}, "incomplete"},
		{"other host", &model.RunCoordinator{Host: "there", PID: 1}, "another host"},
		{"missing namespace", &model.RunCoordinator{Host: "here", PID: 1}, "cannot be verified"},
		{"other namespace", &model.RunCoordinator{Host: "here", PID: 1, PIDNamespace: "other"}, "another PID namespace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, reason := coordinatorLocation(test.identity, "here", "local")
			if got != CoordinatorUnknown || !strings.Contains(reason, test.want) {
				t.Fatalf("assessment = %s (%s), want unknown (%s)", got, reason, test.want)
			}
		})
	}
}

func TestAssessCoordinatorWithoutRunLock(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if state.PIDNamespace() == "" || state.ProcessStart(os.Getpid()) == "" {
		t.Skip("process identity unavailable")
	}
	store := state.NewStore(0o700, 0o600)
	runDir := t.TempDir()
	identity := &model.RunCoordinator{
		Host: host, PID: os.Getpid(), PIDNamespace: state.PIDNamespace(),
		ProcessStart:         state.ProcessStart(os.Getpid()),
		ManagementFinishedAt: "2026-10-20T00:00:00Z",
	}
	if err := state.SaveContext(store, runDir, model.RunContext{Coordinator: identity}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(state.ContextPath(runDir))
	if err != nil {
		t.Fatal(err)
	}
	result, err := AssessCoordinator(store, runDir)
	if err != nil || result.State != CoordinatorLive || result.ObservedAt == "" || result.Identity == nil {
		t.Fatalf("assessment = %+v, %v; completion marker must not hide a live process", result, err)
	}
	after, err := os.ReadFile(state.ContextPath(runDir))
	if err != nil || string(after) != string(before) {
		t.Fatalf("assessment changed context: %v", err)
	}
	entries, err := os.ReadDir(runDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("assessment created files: %v, %v", entries, err)
	}
}

func TestAssessCoordinatorLegacyAndInvalidContext(t *testing.T) {
	store := state.NewStore(0o700, 0o600)
	for _, test := range []struct {
		name, content string
		wantError     bool
	}{
		{"missing", "", false},
		{"legacy", `{}`, false},
		{"invalid", `{`, true},
		{"invalid coordinator type", `{"coordinator":"broken"}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.content != "" {
				if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(test.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := AssessCoordinator(store, dir)
			if (err != nil) != test.wantError || got.State != CoordinatorUnknown || err == nil && got.Reason == "" {
				t.Fatalf("assessment = %+v, %v", got, err)
			}
		})
	}
}
