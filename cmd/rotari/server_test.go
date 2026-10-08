package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// testProjectPaths returns the paths of project in baseDir and creates its
// directory, as add would.
func testProjectPaths(t *testing.T, baseDir, project string) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return paths
}

// useInProcessSupervisor makes the run commands of this test serve their
// run from a supervisor in the test process, instead of starting the test
// binary as a child, and waits for each one to stop at cleanup.
func useInProcessSupervisor(t *testing.T) {
	t.Helper()
	previous := startSupervisor
	t.Cleanup(func() { startSupervisor = previous })
	startSupervisor = func(paths state.ProjectPaths) (*serverinternal.Client, error) {
		clientConn, serverConn, err := serverinternal.Pipe()
		if err != nil {
			return nil, err
		}
		done := make(chan int, 1)
		go func() { done <- runServer(paths, serverConn) }()
		t.Cleanup(func() {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Log("server did not stop after the run completed")
			}
		})
		client, err := serverinternal.Connect(clientConn, 3*time.Second)
		if errors.Is(err, serverinternal.ErrAlreadyRunning) {
			return nil, alreadyRunningError(paths)
		}
		return client, err
	}
}

// holdSupervisorLease holds paths' supervisor lease from the test process
// until the test ends, as a running supervisor does.
func holdSupervisorLease(t *testing.T, paths state.ProjectPaths) {
	t.Helper()
	release, err := serverinternal.Acquire(paths.ProjectDir, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

func TestCmdRunWithRunIDRejectsRunningProjectBeforeQueueConfirmation(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-running-source"
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "active-run")
	defer os.Remove(paths.LockFile)

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdRun([]string{"--basedir", baseDir, "--project-name", "default", "--run-id", runID})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), `project "default" is running; run is not allowed`) {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, output)
	}
	if strings.Contains(string(output), "queue is not empty") {
		t.Fatalf("cmdRun checked queue before running state: %q", output)
	}
}

func TestRunSnapshotsOnlyExplicitlyLoadedConfig(t *testing.T) {
	baseDir := t.TempDir()
	paths := testProjectPaths(t, baseDir, "demo")
	if err := os.WriteFile(filepath.Join(paths.ProjectDir, "config.yaml"), []byte("run:\n  retry: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selectedConfig := filepath.Join(t.TempDir(), "chosen.toml")
	if err := os.WriteFile(selectedConfig, []byte("[run]\nretry = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	notificationConfig := filepath.Join(paths.ProjectDir, "notifications.toml")
	if err := os.WriteFile(notificationConfig, []byte("[webhook]\nrun_failure = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	useInProcessSupervisor(t)
	oldConfig, oldConfigCommand, oldConfigPath := cliConfig, cliConfigCommand, cliConfigPath
	t.Cleanup(func() { cliConfig, cliConfigCommand, cliConfigPath = oldConfig, oldConfigCommand, oldConfigPath })
	originalStart := startSupervisor
	startSupervisor = func(paths state.ProjectPaths) (*serverinternal.Client, error) {
		// The file was loaded and serialized before supervisor startup. Removing
		// it here must not change or prevent this run's immutable snapshot.
		if err := os.Remove(selectedConfig); err != nil {
			return nil, err
		}
		return originalStart(paths)
	}
	if code := run([]string{"run", "--basedir", baseDir, "--project-name", "demo", "--config", selectedConfig, "--quiet"}); code != 0 {
		t.Fatalf("run exit code = %d", code)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	context, err := state.LoadContext(jsonStore(), filepath.Join(paths.RunsDir, meta.LastRunID))
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{selectedConfig, notificationConfig}
	if !reflect.DeepEqual(context.ConfigPaths, wantPaths) {
		t.Fatalf("config paths = %#v, want %#v", context.ConfigPaths, wantPaths)
	}
	wantFiles := []string{"config.toml", "notifications.toml"}
	if !reflect.DeepEqual(context.ConfigSnapshotFiles, wantFiles) {
		t.Fatalf("snapshot files = %#v, want %#v", context.ConfigSnapshotFiles, wantFiles)
	}
}

func TestCmdRunRejectsRemovedOverwriteOption(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	runID := "source-run"
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o755); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdRun([]string{"--basedir", baseDir, "--project-name", "default", "--run-id", runID, "--overwrite"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "no longer accept --overwrite") {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, output)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil || len(queue.Commands) != 1 || queue.Commands[0].ID != "existing" {
		t.Fatalf("queue changed after rejected overwrite: %+v, %v", queue, err)
	}
}

func TestWatchClientInput(t *testing.T) {
	closed := func() io.Reader { return iotest.ErrReader(errors.New("closed")) }
	for _, test := range []struct {
		name       string
		input      io.Reader
		action     string
		wantDetach bool
		wantCancel bool
	}{
		{name: "Ctrl-D EOF detaches under detach policy", input: strings.NewReader(""), action: serverinternal.DisconnectActionDetach, wantDetach: true},
		{name: "Ctrl-D EOF detaches under cancel policy", input: strings.NewReader(""), action: serverinternal.DisconnectActionCancel, wantDetach: true},
		{name: "detach byte after other input", input: io.MultiReader(strings.NewReader("\n"), strings.NewReader("x\x04")), action: serverinternal.DisconnectActionCancel, wantDetach: true},
		{name: "lost terminal detaches by default", input: closed(), action: serverinternal.DisconnectActionDetach, wantDetach: true},
		{name: "lost terminal can cancel", input: closed(), action: serverinternal.DisconnectActionCancel, wantCancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			detach := make(chan struct{}, 1)
			interrupt := make(chan os.Signal, 1)
			watchClientInput(test.input, detach, interrupt, test.action)
			if got := len(detach) == 1; got != test.wantDetach {
				t.Fatalf("detach = %v, want %v", got, test.wantDetach)
			}
			if got := len(interrupt) == 1; got != test.wantCancel {
				t.Fatalf("cancel = %v, want %v", got, test.wantCancel)
			}
		})
	}
}

func TestSendRunRequestQuietSuppressesProgress(t *testing.T) {
	clientConn, conn := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		defer conn.Close()
		if err := serverinternal.Ready(conn, nil); err != nil {
			serverDone <- err
			return
		}
		var request serverinternal.Request
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			serverDone <- err
			return
		}
		if !request.Quiet {
			serverDone <- fmt.Errorf("request.Quiet = false, want true")
			return
		}
		if err := json.NewEncoder(conn).Encode(serverinternal.Response{Progress: true, Message: "=== Run started ===\n  Project: demo"}); err != nil {
			serverDone <- err
			return
		}
		if err := json.NewEncoder(conn).Encode(serverinternal.Response{OK: true, Message: "=== Run finished ===\n  Project: demo\n  Exit code: 0", ExitCode: 0}); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	client, err := serverinternal.Connect(clientConn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := sendRunRequest(client, serverinternal.Request{Op: serverinternal.OpRun, QueueName: "demo", Quiet: true})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil || response.ExitCode != 0 || response.Message != "=== Run finished ===\n  Project: demo\n  Exit code: 0" {
		t.Fatalf("sendRunRequest err=%v response=%#v stdout=%q", err, response, output)
	}
	if len(output) != 0 {
		t.Fatalf("quiet run printed stdout=%q", output)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestCmdRunRejectsMalformedAttemptID(t *testing.T) {
	code := cmdRun([]string{"--job-id", "att_not-an-attempt"})
	if code != 1 {
		t.Fatalf("cmdRun exit code = %d, want 1", code)
	}
	code = cmdRetry([]string{"--job-id", "att_not-an-attempt"})
	if code != 1 {
		t.Fatalf("cmdRetry exit code = %d, want 1", code)
	}
}

func TestCmdRunRejectsAttemptIDFromAnotherRun(t *testing.T) {
	attemptID := makeAttemptID("20260922-000000-00000000", "job-1", 0)
	code := cmdRun([]string{"--run-id", "other-run", "--job-id", attemptID})
	if code != 1 {
		t.Fatalf("cmdRun exit code = %d, want 1", code)
	}
}

func TestCmdServerShutdownFailsWithoutRunningServer(t *testing.T) {
	baseDir := t.TempDir()

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdServerShutdown([]string{"--basedir", baseDir})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "server is not running") {
		t.Fatalf("cmdServerShutdown exit code = %d, stderr = %q", code, output)
	}
}

func TestCancelJobsRejectsJobTraversal(t *testing.T) {
	if _, err := jobController().CancelJobs(t.TempDir(), "default", "run-1", []string{"../outside"}); err == nil {
		t.Fatal("cancelJobs accepted unsafe job ID")
	}
}

func TestCancelJobsRejectsAbsoluteAndNestedJobIDs(t *testing.T) {
	for _, jobID := range []string{"/tmp/outside", "nested/job", "job/..", "job/.", "job/with/slash"} {
		if _, err := jobController().CancelJobs(t.TempDir(), "default", "run-1", []string{jobID}); err == nil {
			t.Fatalf("cancelJobs accepted unsafe job ID %q", jobID)
		}
	}
}

func TestCancelQueueJobsRejectsUnsafeRunIDFromLock(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{RunID: "../outside"}); err != nil {
		t.Fatal(err)
	}

	if _, err := jobController().Cancel(baseDir, "default", "", nil, false); err == nil {
		t.Fatal("Cancel accepted unsafe run ID from lock")
	}
}

func TestControlQueueJobsRejectsUnsafeRunIDFromLock(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{RunID: "nested/run"}); err != nil {
		t.Fatal(err)
	}

	if _, err := jobController().Control(baseDir, "default", "", nil, "suspend"); err == nil {
		t.Fatal("Control accepted unsafe run ID from lock")
	}
}

func TestCancelJobsCancelsSelectedLSFJob(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "bkill-args")
	writeExecutable(t, binDir, "bkill", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\n", argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := struct {
		Executor string `json:"executor"`
		JobID    string `json:"job_id"`
		LSFJobID string `json:"lsf_job_id"`
	}{Executor: "lsf", JobID: "job-1", LSFJobID: "123"}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		t.Fatal(err)
	}

	message, err := jobController().CancelJobs(runDir, "default", "run-1", []string{"job-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "Jobs: 1") {
		t.Fatalf("message = %q, want one cancelled job", message)
	}
	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "123\n" {
		t.Fatalf("bkill arguments = %q, want 123", args)
	}
}

func TestCancelJobsCancelsSelectedPBSJob(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "qdel-args")
	writeExecutable(t, binDir, "qdel", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\n", argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := struct {
		Executor string `json:"executor"`
		JobID    string `json:"job_id"`
		PBSJobID string `json:"pbs_job_id"`
	}{Executor: "pbs", JobID: "job-1", PBSJobID: "123.headnode"}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		t.Fatal(err)
	}

	message, err := jobController().CancelJobs(runDir, "default", "run-1", []string{"job-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "Jobs: 1") {
		t.Fatalf("message = %q, want one cancelled job", message)
	}
	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "123.headnode\n" {
		t.Fatalf("qdel arguments = %q, want 123.headnode", args)
	}
}

func TestCancelJobsCancelsSelectedSlurmJob(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "scancel-args")
	writeExecutable(t, binDir, "scancel", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\n", argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := struct {
		Executor   string `json:"executor"`
		JobID      string `json:"job_id"`
		SlurmJobID string `json:"slurm_job_id"`
	}{Executor: "slurm", JobID: "job-1", SlurmJobID: "12345"}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		t.Fatal(err)
	}

	message, err := jobController().CancelJobs(runDir, "default", "run-1", []string{"job-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "Jobs: 1") {
		t.Fatalf("message = %q, want one cancelled job", message)
	}
	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "12345\n" {
		t.Fatalf("scancel arguments = %q, want 12345", args)
	}
}

// Regression test: a whole-project cancel (no --job-id) must reach an
// already-submitted Slurm job even mid-run, before the aggregate
// slurm_jobs.json snapshot exists (it is only written once the whole run
// finishes).
func TestCancelQueueCancelsRunningSlurmJobMidRun(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "scancel-args")
	writeExecutable(t, binDir, "scancel", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" >> %q\n", argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "job-1", Command: []string{"echo", "hi"}},
		{ID: "job-2", Command: []string{"echo", "hi"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	// job-1 is currently running under Slurm; job-2 has not been submitted
	// yet (no job directory at all). Neither has slurm_jobs.json, which is
	// only written after the whole run completes.
	job1Dir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(job1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(job1Dir, "job.json"), struct {
		Executor   string `json:"executor"`
		JobID      string `json:"job_id"`
		SlurmJobID string `json:"slurm_job_id"`
	}{Executor: "slurm", JobID: "job-1", SlurmJobID: "12345"}); err != nil {
		t.Fatal(err)
	}

	if _, err := jobController().Cancel(baseDir, "default", "", nil, false); err != nil {
		t.Fatal(err)
	}

	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "12345\n" {
		t.Fatalf("scancel arguments = %q, want 12345", args)
	}
	if _, err := os.Stat(filepath.Join(runDir, "job-2", "cancelled")); err != nil {
		t.Fatalf("job-2 was not marked cancelled before submission: %v", err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "cancelling" {
		t.Fatalf("meta.Phase = %q, want cancelling", meta.Phase)
	}
}

func TestControlQueueJobsControlsSelectedSlurmJob(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "scontrol-args")
	writeExecutable(t, binDir, "scontrol", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> %q\n", argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(paths.RunsDir, "run-1", "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), struct {
		Executor   string `json:"executor"`
		JobID      string `json:"job_id"`
		SlurmJobID string `json:"slurm_job_id"`
	}{Executor: "slurm", JobID: "job-1", SlurmJobID: "12345"}); err != nil {
		t.Fatal(err)
	}

	if _, err := jobController().Control(baseDir, "default", "", []string{"job-1"}, "suspend"); err != nil {
		t.Fatal(err)
	}
	if _, err := jobController().Control(baseDir, "default", "", []string{"job-1"}, "resume"); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "suspend 12345\nresume 12345\n" {
		t.Fatalf("scontrol arguments = %q, want suspend and resume for 12345", args)
	}
}

func TestControlQueueJobsReportsMissingScontrolBinary(t *testing.T) {
	emptyBinDir := t.TempDir()
	t.Setenv("PATH", emptyBinDir)

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(paths.RunsDir, "run-1", "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), struct {
		Executor   string `json:"executor"`
		JobID      string `json:"job_id"`
		SlurmJobID string `json:"slurm_job_id"`
	}{Executor: "slurm", JobID: "job-1", SlurmJobID: "12345"}); err != nil {
		t.Fatal(err)
	}

	_, err = jobController().Control(baseDir, "default", "", []string{"job-1"}, "suspend")
	if err == nil {
		t.Fatal("suspend without scontrol on PATH unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "not installed on this host") {
		t.Fatalf("error = %q, want a hint that scontrol is missing on this host", err)
	}
}

func TestControlQueueJobsSurfacesScontrolRejectionForPendingJob(t *testing.T) {
	binDir := t.TempDir()
	// A Slurm job that is still queued (PENDING), not yet running, rejects
	// scontrol suspend with a message on stderr; that explanation must reach
	// the caller instead of a bare "exit status 1".
	writeExecutable(t, binDir, "scontrol", "#!/bin/sh\necho 'slurm_suspend error: Job is not running' >&2\nexit 1\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(paths.RunsDir, "run-1", "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), struct {
		Executor   string `json:"executor"`
		JobID      string `json:"job_id"`
		SlurmJobID string `json:"slurm_job_id"`
	}{Executor: "slurm", JobID: "job-1", SlurmJobID: "12345"}); err != nil {
		t.Fatal(err)
	}

	_, err = jobController().Control(baseDir, "default", "", []string{"job-1"}, "suspend")
	if err == nil {
		t.Fatal("suspend of a pending job unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "slurm_suspend error: Job is not running") {
		t.Fatalf("error = %q, want it to include scontrol's own explanation", err)
	}
}

func TestCancelJobsReportsMissingScancelBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), struct {
		Executor   string `json:"executor"`
		JobID      string `json:"job_id"`
		SlurmJobID string `json:"slurm_job_id"`
	}{Executor: "slurm", JobID: "job-1", SlurmJobID: "12345"}); err != nil {
		t.Fatal(err)
	}

	_, err := jobController().CancelJobs(runDir, "default", "run-1", []string{"job-1"})
	if err == nil {
		t.Fatal("cancel without scancel on PATH unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "not installed on this host") {
		t.Fatalf("error = %q, want a hint that scancel is missing on this host", err)
	}
}

func TestCmdAddThenCmdRunExecutesLocalJobEndToEnd(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	// A completed sync run drops activeRuns to zero, which makes the server
	// stop itself; no explicit shutdown request is needed here.
	useInProcessSupervisor(t)

	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "echo", "hello"}); code != 0 {
		t.Fatalf("cmdAdd exit code = %d, want 0", code)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdRun([]string{"--basedir", baseDir, "--project-name", "demo"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Run finished") {
		t.Fatalf("cmdRun exit code = %d, stdout = %q", code, output)
	}

	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "finished" || meta.LastRunExitCode != 0 {
		t.Fatalf("meta = %#v, want finished run with exit code 0", meta)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands = %#v, want empty queue after run", queue.Commands)
	}
}

func TestCmdRunFailedRestoresEmptyQueue(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-failed-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	sourceRunID := makeRunID()
	marker := filepath.Join(baseDir, "executed")
	if err := writeJSON(filepath.Join(paths.RunsDir, sourceRunID, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "success", Command: []string{"sh", "-c", fmt.Sprintf("printf success >> %q", marker)}},
		{ID: "failed", Command: []string{"sh", "-c", fmt.Sprintf("printf failed >> %q", marker)}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, sourceRunID, "summary.json"), model.RunSummary{RunID: sourceRunID, Results: []model.JobResult{
		{ID: "success", ExitCode: 0},
		{ID: "failed", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: sourceRunID}); err != nil {
		t.Fatal(err)
	}

	useInProcessSupervisor(t)

	if code := cmdRun([]string{"--basedir", baseDir, "--project-name", "demo", "--failed", "--quiet"}); code != 0 {
		t.Fatalf("cmdRun --failed exit code = %d, want 0", code)
	}
	output, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "failed" {
		t.Fatalf("executed jobs = %q, want only failed", output)
	}
}

func TestCmdRunWithAttemptIDCopiesAndExecutesSourceAttempt(t *testing.T) {
	testCmdWithAttemptID(t, false)
}

func TestCmdRetryWithAttemptIDCopiesAndExecutesSourceAttempt(t *testing.T) {
	testCmdWithAttemptID(t, true)
}

func testCmdWithAttemptID(t *testing.T, retry bool) {
	baseDir, err := os.MkdirTemp("", "rotari-attempt-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, defaultMeta()); err != nil {
		t.Fatal(err)
	}
	sourceRunID := makeRunID()
	attemptID := makeAttemptID(sourceRunID, "source", 0)
	sourceRunDir := filepath.Join(paths.RunsDir, sourceRunID)
	if err := writeJSON(filepath.Join(sourceRunDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "source", Command: []string{"printf", "attempt-source"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(sourceRunDir, "summary.json"), model.RunSummary{RunID: sourceRunID, Status: "failed", Results: []model.JobResult{{ID: "source", AttemptID: attemptID, ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}
	attemptDir, err := specificAttemptJobDir(sourceRunDir, "source", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, sourceRunID); err != nil {
		t.Fatal(err)
	}

	useInProcessSupervisor(t)
	args := []string{"--basedir", baseDir, "--project-name", "demo", "--job-id", attemptID}
	if retry {
		if code := cmdRetry(args); code != 0 {
			t.Fatalf("cmdRetry exit code = %d", code)
		}
	} else if code := cmdRun(args); code != 0 {
		t.Fatalf("cmdRun exit code = %d", code)
	}
	paths, err = state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastRunID == sourceRunID || meta.Phase != "finished" {
		t.Fatalf("meta = %#v, want a new finished run", meta)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue = %#v, want empty queue", queue.Commands)
	}
}

func TestServerLoggerCapsFileSize(t *testing.T) {
	logger := newServerLogger(testProjectPaths(t, t.TempDir(), "default"))
	logger.Writef("%s", strings.Repeat("x", maxServerLogSize))
	logger.Writef("latest event")

	data, err := os.ReadFile(logger.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > maxServerLogSize {
		t.Fatalf("server log size = %d, want at most %d", len(data), maxServerLogSize)
	}
	if !strings.Contains(string(data), "latest event") {
		t.Fatalf("server log does not contain latest event")
	}
}

func TestServerHandleRejectsMalformedJSON(t *testing.T) {
	client, serverConn := net.Pipe()
	defer client.Close()

	server := newRotariServer(testProjectPaths(t, t.TempDir(), "default"))
	go server.Handle(serverConn)

	if _, err := client.Write([]byte("{invalid}\n")); err != nil {
		t.Fatal(err)
	}
	var response serverinternal.Response
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || !strings.Contains(response.Message, "invalid character") {
		t.Fatalf("response = %+v, want JSON decoding error", response)
	}
}

func TestServerHandleRejectsUnknownOperation(t *testing.T) {
	client, serverConn := net.Pipe()
	defer client.Close()

	server := newRotariServer(testProjectPaths(t, t.TempDir(), "default"))
	go server.Handle(serverConn)
	if err := json.NewEncoder(client).Encode(serverinternal.Request{Op: "unknown"}); err != nil {
		t.Fatal(err)
	}
	var response serverinternal.Response
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Message != "unknown server operation: unknown" {
		t.Fatalf("response = %+v, want unknown-operation error", response)
	}
}

// TestCmdCancelRejectsWholeRunFromWrongHostViaCLI exercises the actual CLI
// entry point rather than calling jobController().Cancel directly, so the
// host-mismatch guard is verified on the same code path a real `rotari
// cancel` invocation uses.
func TestCmdCancelRejectsWholeRunFromWrongHostViaCLI(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-cli-host-mismatch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })

	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	// A PID that isn't this test process's own, recorded as owned by
	// another host, mirrors a runner that is genuinely still active
	// elsewhere over a shared base directory.
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid() + 1, RunID: "run-1", StartedAt: nowRFC3339(), Host: "other-host"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdCancel([]string{"--basedir", baseDir, "--project-name", "default"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("cmdCancel exit = %d, want 1; stderr=%s", code, output)
	}
	if !strings.Contains(string(output), "other-host") {
		t.Fatalf("stderr = %q, want it to mention the recorded host", output)
	}
}

// TestCmdCancelAcceptsPositionalJobID exercises the CLI entry point with a
// positional job ID (instead of --job-id), checking that it cancels the
// not-yet-submitted job without a server.
func TestCmdCancelAcceptsPositionalJobID(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-cli-positional-job-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })

	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"echo", "hi"}}}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdCancel([]string{"--basedir", baseDir, "--project-name", "default", "job-1"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdCancel exit = %d, want 0; stdout=%s", code, output)
	}
	if !strings.Contains(string(output), "Jobs: 1") {
		t.Fatalf("stdout = %q, want it to report one cancelled job", output)
	}
	if _, err := os.Stat(filepath.Join(runDir, "job-1", "cancelled")); err != nil {
		t.Fatalf("job-1 was not marked cancelled: %v", err)
	}
}

// TestCmdCancelAcceptsPositionalRunID exercises the CLI entry point with a
// bare run ID resolved through the run registry, checking that it locates the
// run's project the same way an "att_" attempt ID does.
func TestCmdCancelAcceptsPositionalRunID(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir, err := os.MkdirTemp("", "rotari-cli-positional-run-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })

	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260922-000000-00000000"
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid() + 1, RunID: runID, StartedAt: nowRFC3339(), Host: "other-host"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := registerRunLocation(runLocation{BaseDir: baseDir, ProjectName: "default", RunID: runID}); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdCancel([]string{"--project-name", "default", runID})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("cmdCancel exit = %d, want 1; stderr=%s", code, output)
	}
	if !strings.Contains(string(output), "other-host") {
		t.Fatalf("stderr = %q, want it to mention the recorded host", output)
	}
}

func TestCmdServerStatusReportsRunningServer(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-status-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })
	holdSupervisorLease(t, testProjectPaths(t, baseDir, "live"))
	testProjectPaths(t, baseDir, "idle")

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdServerStatus([]string{"--basedir", baseDir})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdServerStatus exit code = %d, want 0", code)
	}
	for _, want := range []string{"server is running", "project=live", baseDir} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("server status does not contain %q: %s", want, output)
		}
	}
	if strings.Contains(string(output), "project=idle") {
		t.Fatalf("server status reports a project without a supervisor: %s", output)
	}
}

func TestCmdServerStatusReportsStoppedServer(t *testing.T) {
	baseDir := t.TempDir()
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdServerStatus([]string{"--basedir", baseDir})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "server is not running") {
		t.Fatalf("cmdServerStatus exit code = %d, stderr = %q", code, output)
	}
}

func TestCmdServerListReportsLiveServer(t *testing.T) {
	masterDir := t.TempDir()
	baseDir, err := os.MkdirTemp("", "rotari-list-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })
	holdSupervisorLease(t, testProjectPaths(t, baseDir, "live"))
	if err := registerServer(masterDir, serverRecord{BaseDir: baseDir, Project: "live", PID: os.Getpid(), LastSeen: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdServerList([]string{"--masterdir", masterDir})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdServerList exit code = %d, want 0", code)
	}
	for _, want := range []string{"master=" + masterDir, "PID", "PROJECT", "live", baseDir} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("server list does not contain %q: %s", want, output)
		}
	}
}

func TestListServersKeepsLiveAndRemovesInvalidRecords(t *testing.T) {
	masterDir := t.TempDir()
	liveBaseDir, err := os.MkdirTemp("", "rotari-live-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(liveBaseDir) })
	holdSupervisorLease(t, testProjectPaths(t, liveBaseDir, "live"))

	live := serverRecord{BaseDir: liveBaseDir, Project: "live", PID: os.Getpid(), StartedAt: nowRFC3339(), LastSeen: nowRFC3339()}
	if err := registerServer(masterDir, live); err != nil {
		t.Fatal(err)
	}
	staleBaseDir := filepath.Join(t.TempDir(), "stale")
	if err := registerServer(masterDir, serverRecord{BaseDir: staleBaseDir, Project: "p", PID: 999999}); err != nil {
		t.Fatal(err)
	}
	// A record from before supervisors were per project names no project.
	unversionedPath := serverRecordPath(masterDir, liveBaseDir, "")
	if err := registerServer(masterDir, serverRecord{BaseDir: liveBaseDir, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	malformedPath := filepath.Join(masterDir, "malformed.json")
	if err := os.WriteFile(malformedPath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	servers, err := listServers(masterDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].BaseDir != liveBaseDir || servers[0].Project != "live" {
		t.Fatalf("servers = %+v, want only project live of %q", servers, liveBaseDir)
	}
	for _, path := range []string{serverRecordPath(masterDir, staleBaseDir, "p"), unversionedPath, malformedPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid record %q was not removed: %v", path, err)
		}
	}
}

func TestServerRegistryRecordLifecycle(t *testing.T) {
	masterDir := t.TempDir()
	baseDir := t.TempDir()
	record := serverRecord{
		BaseDir: baseDir, Project: "demo", PID: 1234, StartedAt: "2026-01-01T00:00:00Z", LastSeen: "2026-01-01T00:00:00Z",
	}
	if err := registerServer(masterDir, record); err != nil {
		t.Fatal(err)
	}
	if err := touchServerRecord(masterDir, baseDir, "demo"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(serverRecordPath(masterDir, baseDir, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	var updated serverRecord
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.LastSeen == record.LastSeen {
		t.Fatalf("last seen was not updated: %+v", updated)
	}
	formatted := formatServerList([]serverRecord{updated})
	for _, want := range []string{"PID", "LAST_SEEN", "PROJECT", "demo", baseDir, "1234"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted server list does not contain %q:\n%s", want, formatted)
		}
	}
	if got := formatServerList(nil); got != "no running servers" {
		t.Fatalf("empty server list = %q", got)
	}
	if err := unregisterServer(masterDir, baseDir, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := unregisterServer(masterDir, baseDir, "demo"); err != nil {
		t.Fatalf("second unregister failed: %v", err)
	}
}

func TestRunServerLifecycle(t *testing.T) {
	baseDir := t.TempDir()
	paths := testProjectPaths(t, baseDir, "demo")
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	clientConn, serverConn, err := serverinternal.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		result <- runServer(paths, serverConn)
	}()
	client, err := serverinternal.Connect(clientConn, 3*time.Second)
	if err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}
	if client.PID != os.Getpid() {
		t.Fatalf("ready PID = %d, want %d", client.PID, os.Getpid())
	}
	if _, err := os.Stat(serverRecordPath(masterDir, baseDir, "demo")); err != nil {
		t.Fatalf("server registry record missing: %v", err)
	}
	for _, path := range []string{serverinternal.LockPath(paths.ProjectDir), serverinternal.PIDPath(paths.ProjectDir)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("supervisor file %q is not in the project directory: %v", path, err)
		}
	}
	if pid, running := serverinternal.Running(paths.ProjectDir); !running || pid != os.Getpid() {
		t.Fatalf("Running() = %d, %v, want the supervisor", pid, running)
	}

	// A second supervisor of the project reports the first instead of
	// serving.
	secondClient, secondServer, err := serverinternal.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = runServer(paths, secondServer) }()
	if _, err := serverinternal.Connect(secondClient, 3*time.Second); !errors.Is(err, serverinternal.ErrAlreadyRunning) {
		t.Fatalf("second supervisor error = %v, want ErrAlreadyRunning", err)
	}

	// A client that leaves without a run request stops its supervisor.
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-result:
		if code != 0 {
			t.Fatalf("runServer exit code = %d, want 0", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop after its client left")
	}
	for _, path := range []string{serverinternal.PIDPath(paths.ProjectDir), serverRecordPath(masterDir, baseDir, "demo")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("server artifact %q remains after it stopped: %v", path, err)
		}
	}
	if _, running := serverinternal.Running(paths.ProjectDir); running {
		t.Fatal("lease is still held after the server stopped")
	}
}
