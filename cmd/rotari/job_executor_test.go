package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSchedulerStatusRoundTripNormalizesState(t *testing.T) {
	jobDir := t.TempDir()
	writeSchedulerStatus(jobDir, "  RUNNING  ")

	if state := loadSchedulerStatus(jobDir); state != "running" {
		t.Fatalf("scheduler state = %q, want running", state)
	}
}

func TestWriteSchedulerStatusIgnoresEmptyState(t *testing.T) {
	jobDir := t.TempDir()
	writeSchedulerStatus(jobDir, "  ")

	if _, err := os.Stat(filepath.Join(jobDir, "scheduler_status.json")); !os.IsNotExist(err) {
		t.Fatalf("empty scheduler status created a file: %v", err)
	}
}

func TestLoadSchedulerStatusIgnoresMalformedFile(t *testing.T) {
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "scheduler_status.json"), []byte("{invalid}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if state := loadSchedulerStatus(jobDir); state != "" {
		t.Fatalf("scheduler state = %q, want empty state", state)
	}
}

func TestExecutorRunSettingsOverrideDispatchDefaults(t *testing.T) {
	settings := executorRunSettingsMap{
		"ssh": {Concurrency: 3, Options: []string{"builder@worker-01", "-p", "2222"}},
	}

	if got := effectiveExecutorConcurrency(settings, "ssh", 8); got != 3 {
		t.Fatalf("SSH concurrency = %d, want 3", got)
	}
	if got := effectiveExecutorConcurrency(settings, "slurm", 8); got != 8 {
		t.Fatalf("Slurm concurrency = %d, want common default 8", got)
	}
	if got := effectiveExecutorOptions(settings, "ssh", []string{"common"}); len(got) != 3 || got[0] != "builder@worker-01" {
		t.Fatalf("SSH options = %#v, want executor-specific options", got)
	}
	if got := effectiveExecutorOptions(settings, "slurm", []string{"common"}); len(got) != 1 || got[0] != "common" {
		t.Fatalf("Slurm options = %#v, want common options", got)
	}
}
