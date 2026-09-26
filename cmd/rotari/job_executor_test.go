package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

func writeSchedulerStatus(jobDir, state string) {
	executor.WriteSchedulerStatus(jsonStore(), jobDir, state, time.Now())
}

func loadSchedulerStatus(jobDir string) string {
	return executor.LoadSchedulerStatus(jsonStore(), jobDir)
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
	queue := model.Queue{Commands: []model.QueuedCommand{{
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
		queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Executor: "slurm", Command: []string{"true"}}}}
		if err := validateLocalExecutionEnvironment(queue); err == nil || !strings.Contains(err.Error(), `slurm executor command "sbatch" is not available`) {
			t.Fatalf("missing scheduler command error = %v", err)
		}
	})

	t.Run("ssh", func(t *testing.T) {
		oldSSHCommandPath := executor.SSHCommandPath
		executor.SSHCommandPath = filepath.Join(t.TempDir(), "missing-ssh")
		t.Cleanup(func() { executor.SSHCommandPath = oldSSHCommandPath })
		queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Executor: "ssh", ExecutorOptions: []string{"worker.example"}, Command: []string{"true"}}}}
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
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID:               "job-1",
		Executor:         "slurm",
		Command:          []string{"remote-command"},
		WorkingDirectory: "/remote/path/not/available/here",
	}}}
	if err := validateLocalExecutionEnvironment(queue); err != nil {
		t.Fatalf("remote working directory was checked locally: %v", err)
	}
}
