package executor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSchedulerStatusRoundTripNormalizesState(t *testing.T) {
	store := state.NewStore(0o700, 0o600)
	jobDir := t.TempDir()
	WriteSchedulerStatus(store, jobDir, "  RUNNING  ", time.Now())

	if got := LoadSchedulerStatus(store, jobDir); got != "running" {
		t.Fatalf("scheduler state = %q, want running", got)
	}
}

func TestWriteSchedulerStatusIgnoresEmptyState(t *testing.T) {
	store := state.NewStore(0o700, 0o600)
	jobDir := t.TempDir()
	WriteSchedulerStatus(store, jobDir, "  ", time.Now())

	if _, err := os.Stat(filepath.Join(jobDir, "scheduler_status.json")); !os.IsNotExist(err) {
		t.Fatalf("empty scheduler status created a file: %v", err)
	}
}

func TestLoadSchedulerStatusIgnoresMalformedFile(t *testing.T) {
	store := state.NewStore(0o700, 0o600)
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "scheduler_status.json"), []byte("{invalid}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := LoadSchedulerStatus(store, jobDir); got != "" {
		t.Fatalf("scheduler state = %q, want empty state", got)
	}
}

func TestSchedulerStateTerminalAndExitCode(t *testing.T) {
	tests := []struct {
		state    string
		terminal bool
		exitCode int
		finished bool
	}{
		{state: "RUN", terminal: false},
		{state: "completed", terminal: true, exitCode: 0, finished: true},
		{state: " finished ", terminal: true, exitCode: 0, finished: true},
		{state: "failed", terminal: true, exitCode: 1, finished: true},
		{state: "unknown", terminal: true, exitCode: 1, finished: true},
	}
	for _, test := range tests {
		if got := SchedulerStateTerminal(test.state); got != test.terminal {
			t.Errorf("SchedulerStateTerminal(%q) = %t, want %t", test.state, got, test.terminal)
		}
		got, ok := SchedulerStateExitCode(test.state)
		if ok != test.finished || (ok && got != test.exitCode) {
			t.Errorf("SchedulerStateExitCode(%q) = %d, %t, want %d, %t", test.state, got, ok, test.exitCode, test.finished)
		}
	}
}
