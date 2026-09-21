package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestValidateQueueForRunRejectsInvalidExecutorConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		queue Queue
		want  string
	}{
		{
			name:  "unknown default executor",
			queue: Queue{DefaultExecutor: "unknown", Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}},
			want:  "unsupported executor: unknown",
		},
		{
			name:  "unknown job executor",
			queue: Queue{Commands: []QueuedCommand{{ID: "job-1", Executor: "unknown", Command: []string{"true"}}}},
			want:  `job "job-1" uses unsupported executor: unknown`,
		},
		{
			name:  "invalid scheduler option quoting",
			queue: Queue{Commands: []QueuedCommand{{ID: "job-1", Executor: "slurm", ExecutorOptions: []string{`"unterminated`}, Command: []string{"true"}}}},
			want:  `job "job-1" executor options: invalid executor option`,
		},
		{
			name:  "missing SSH target",
			queue: Queue{Commands: []QueuedCommand{{ID: "job-1", Executor: "ssh", Command: []string{"true"}}}},
			want:  `job "job-1" executor options: SSH executor requires its first executor option to be the target host`,
		},
		{
			name:  "reserved Slurm array option",
			queue: Queue{Commands: []QueuedCommand{{ID: "job-1", Executor: "slurm", ExecutorOptions: []string{"--array=1-2"}, Array: &ArraySpec{First: 1, Last: 2}, Command: []string{"true"}}}},
			want:  `job "job-1" executor options: executor options must not include --array`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateQueueForRun(test.queue, "", nil, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateQueueForRun error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateQueueForRunUsesDispatchOptionPriority(t *testing.T) {
	queue := Queue{
		DefaultExecutorOptions: []string{"invalid-default"},
		Commands: []QueuedCommand{{
			ID:              "job-1",
			Executor:        "ssh",
			ExecutorOptions: []string{"worker.example"},
			Command:         []string{"true"},
		}},
	}
	settings := executorRunSettingsMap{"ssh": {Options: []string{"invalid-setting"}}}
	if err := validateQueueForRun(queue, "", []string{"invalid-common"}, settings); err != nil {
		t.Fatalf("validateQueueForRun rejected job-specific SSH target: %v", err)
	}
}

func TestValidateLocalExecutionEnvironmentForLocalJob(t *testing.T) {
	workingDirectory := t.TempDir()
	binDir := filepath.Join(workingDirectory, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "worker"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{
		ID:               "job-1",
		Command:          []string{"worker"},
		WorkingDirectory: workingDirectory,
		Environment:      []string{"PATH=bin"},
	}}}
	if err := validateLocalExecutionEnvironment(queue); err != nil {
		t.Fatalf("valid local environment rejected: %v", err)
	}

	queue.Commands[0].WorkingDirectory = filepath.Join(workingDirectory, "missing")
	if err := validateLocalExecutionEnvironment(queue); err == nil || !strings.Contains(err.Error(), "working directory") {
		t.Fatalf("missing working directory error = %v", err)
	}
}

func TestValidateLocalExecutionEnvironmentRequiresExecutorCommands(t *testing.T) {
	t.Run("scheduler", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Executor: "slurm", Command: []string{"true"}}}}
		if err := validateLocalExecutionEnvironment(queue); err == nil || !strings.Contains(err.Error(), `slurm executor command "sbatch" is not available`) {
			t.Fatalf("missing scheduler command error = %v", err)
		}
	})

	t.Run("ssh", func(t *testing.T) {
		oldSSHCommandPath := sshCommandPath
		sshCommandPath = filepath.Join(t.TempDir(), "missing-ssh")
		t.Cleanup(func() { sshCommandPath = oldSSHCommandPath })
		queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Executor: "ssh", ExecutorOptions: []string{"worker.example"}, Command: []string{"true"}}}}
		if err := validateLocalExecutionEnvironment(queue); err == nil || !strings.Contains(err.Error(), "ssh executor command") {
			t.Fatalf("missing SSH command error = %v", err)
		}
	})
}

func TestValidateLocalExecutionEnvironmentDoesNotStatRemoteWorkingDirectory(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "sbatch"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	queue := Queue{Commands: []QueuedCommand{{
		ID:               "job-1",
		Executor:         "slurm",
		Command:          []string{"remote-command"},
		WorkingDirectory: "/remote/path/not/available/here",
	}}}
	if err := validateLocalExecutionEnvironment(queue); err != nil {
		t.Fatalf("remote working directory was checked locally: %v", err)
	}
}
