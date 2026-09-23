package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWaitForAsyncRunCallsOnDone(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	waitForAsyncRun(command, func() { close(done) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("async run completion callback was not called")
	}
}

func TestCmdRunWithRunIDRejectsRunningProjectBeforeQueueConfirmation(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-running-source"
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := acquireLock(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "active-run"}); err != nil {
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

func TestCmdRunOverwriteSkipsQueueConfirmation(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
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
	if code != 1 || !strings.Contains(string(output), "command snapshot has no jobs") {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, output)
	}
	if strings.Contains(string(output), "queue is not empty") {
		t.Fatalf("cmdRun prompted instead of honoring --overwrite: %q", output)
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

func TestEnqueueCommandRejectsInterruptedRunWithoutChangingQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	original := Queue{Commands: []QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}
	if err := writeJSON(paths.QueueFile, original); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "interrupted-run"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "interrupted-run")

	_, err = enqueueCommand(baseDir, "default", []string{"duplicate"}, "", nil, nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), `project "default" has interrupted run "interrupted-run"; add is not allowed`) {
		t.Fatalf("enqueueCommand error = %v, want interrupted run error", err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "existing" {
		t.Fatalf("queue changed after rejected add: %#v", queue.Commands)
	}
}

func TestCmdAddEnqueuesJob(t *testing.T) {
	baseDir := t.TempDir()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdAdd([]string{
		"--basedir", baseDir, "--project-name", "demo", "--job-name", "job",
		"--stage", "prepare", "--executor", "local", "--env", "TOKEN=secret", "echo", "hello",
	})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "submitted project=demo") {
		t.Fatalf("cmdAdd exit code = %d, stdout = %q", code, output)
	}

	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "job" || queue.Commands[0].Stage != "prepare" || queue.Commands[0].Executor != "local" ||
		len(queue.Commands[0].Environment) != 1 || queue.Commands[0].Environment[0] != "TOKEN=secret" {
		t.Fatalf("queue commands = %#v, want persisted job with executor and env", queue.Commands)
	}
}

func TestCmdAddRejectsMissingCommand(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo"}); code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1 for missing command", code)
	}
}

func TestCmdAddRejectsInvalidArrayRange(t *testing.T) {
	baseDir := t.TempDir()
	code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--array", "not-a-range", "echo", "hello"})
	if code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1 for invalid array range", code)
	}
}

func TestCmdAddRejectsInvalidEnv(t *testing.T) {
	baseDir := t.TempDir()
	code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--env", "NOVALUE", "echo", "hello"})
	if code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1 for invalid --env", code)
	}
}

func TestCmdAddRejectsDuplicateJobNameWithoutWriting(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "prepare", "echo", "one"}); code != 0 {
		t.Fatalf("first cmdAdd exit code = %d, want 0", code)
	}
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "prepare", "echo", "two"}); code == 0 {
		t.Fatal("cmdAdd accepted a duplicate job name")
	}

	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Command[len(queue.Commands[0].Command)-1] != "one" {
		t.Fatalf("queue commands = %#v, want only the first job (rejected add must not write)", queue.Commands)
	}
}

func TestCmdServerRequestFailsWithoutRunningServer(t *testing.T) {
	baseDir := t.TempDir()

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdServerRequest([]string{"--basedir", baseDir}, "ping")
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "failed to contact server") {
		t.Fatalf("cmdServerRequest exit code = %d, stderr = %q", code, output)
	}
}

func TestCancelJobsRejectsJobTraversal(t *testing.T) {
	if _, err := cancelJobs(t.TempDir(), "default", "run-1", []string{"../outside"}); err == nil {
		t.Fatal("cancelJobs accepted unsafe job ID")
	}
}

func TestCancelJobsRejectsAbsoluteAndNestedJobIDs(t *testing.T) {
	for _, jobID := range []string{"/tmp/outside", "nested/job", "job/..", "job/.", "job/with/slash"} {
		if _, err := cancelJobs(t.TempDir(), "default", "run-1", []string{jobID}); err == nil {
			t.Fatalf("cancelJobs accepted unsafe job ID %q", jobID)
		}
	}
}

func TestCancelQueueJobsRejectsUnsafeRunIDFromLock(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{RunID: "../outside"}); err != nil {
		t.Fatal(err)
	}

	if _, err := cancelQueueJobs(baseDir, "default", nil, false); err == nil {
		t.Fatal("cancelQueueJobs accepted unsafe run ID from lock")
	}
}

func TestControlQueueJobsRejectsUnsafeRunIDFromLock(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{RunID: "nested/run"}); err != nil {
		t.Fatal(err)
	}

	if _, err := controlQueueJobs(baseDir, "default", nil, "suspend"); err == nil {
		t.Fatal("controlQueueJobs accepted unsafe run ID from lock")
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

	message, err := cancelJobs(runDir, "default", "run-1", []string{"job-1"})
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

	message, err := cancelJobs(runDir, "default", "run-1", []string{"job-1"})
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

	message, err := cancelJobs(runDir, "default", "run-1", []string{"job-1"})
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
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{
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

	if _, err := cancelQueue(baseDir, "default", false); err != nil {
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
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
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

	if _, err := controlQueueJobs(baseDir, "default", []string{"job-1"}, "suspend"); err != nil {
		t.Fatal(err)
	}
	if _, err := controlQueueJobs(baseDir, "default", []string{"job-1"}, "resume"); err != nil {
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
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
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

	_, err = controlQueueJobs(baseDir, "default", []string{"job-1"}, "suspend")
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
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
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

	_, err = controlQueueJobs(baseDir, "default", []string{"job-1"}, "suspend")
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

	_, err := cancelJobs(runDir, "default", "run-1", []string{"job-1"})
	if err == nil {
		t.Fatal("cancel without scancel on PATH unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "not installed on this host") {
		t.Fatalf("error = %q, want a hint that scancel is missing on this host", err)
	}
}

func TestCmdAddThenCmdRunExecutesLocalJobEndToEnd(t *testing.T) {
	// A short, non-nested temp dir is required: the unix socket path derived
	// from baseDir must stay under the ~108 byte sun_path limit.
	baseDir, err := os.MkdirTemp("", "rotari-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	serverDone := make(chan int, 1)
	go func() { serverDone <- runServer(baseDir) }()

	deadline := time.Now().Add(3 * time.Second)
	var pingErr error
	for time.Now().Before(deadline) {
		response, err := sendServerRequest(baseDir, serverRequest{Op: "ping"})
		if err == nil && response.OK {
			pingErr = nil
			break
		}
		pingErr = err
		time.Sleep(10 * time.Millisecond)
	}
	if pingErr != nil {
		t.Fatalf("server did not become ready: %v", pingErr)
	}
	// A completed sync run drops activeRuns to zero, which makes the server
	// stop itself; no explicit shutdown request is needed here.
	defer func() {
		select {
		case <-serverDone:
		case <-time.After(3 * time.Second):
			t.Log("server did not stop after the run completed")
		}
	}()

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

	paths, err := resolvePaths(baseDir, "demo")
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
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, defaultMeta()); err != nil {
		t.Fatal(err)
	}
	sourceRunID := makeRunID()
	attemptID := makeAttemptID(sourceRunID, "source", 0)
	sourceRunDir := filepath.Join(paths.RunsDir, sourceRunID)
	if err := writeJSON(filepath.Join(sourceRunDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "source", Command: []string{"printf", "attempt-source"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(sourceRunDir, "summary.json"), RunSummary{RunID: sourceRunID, Status: "failed", Results: []JobResult{{ID: "source", AttemptID: attemptID, ExitCode: 1}}}); err != nil {
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

	serverDone := make(chan int, 1)
	go func() { serverDone <- runServer(baseDir) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if response, pingErr := sendServerRequest(baseDir, serverRequest{Op: "ping"}); pingErr == nil && response.OK {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	args := []string{"--basedir", baseDir, "--project-name", "demo", "--job-id", attemptID}
	if retry {
		if code := cmdRetry(args); code != 0 {
			t.Fatalf("cmdRetry exit code = %d", code)
		}
	} else if code := cmdRun(args); code != 0 {
		t.Fatalf("cmdRun exit code = %d", code)
	}
	paths, err = resolvePaths(baseDir, "demo")
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
	select {
	case <-serverDone:
	case <-time.After(3 * time.Second):
		t.Log("server did not stop after attempt run")
	}
}

func TestServerHandlePing(t *testing.T) {
	client, serverConn := net.Pipe()
	defer client.Close()

	server := &rotariServer{stopped: make(chan struct{}), lastAccess: time.Now()}
	go server.handle(t.TempDir(), serverConn)

	if err := json.NewEncoder(client).Encode(serverRequest{Op: "ping"}); err != nil {
		t.Fatal(err)
	}
	var response serverResponse
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.PID == 0 || response.Protocol != serverProtocolVersion {
		t.Fatalf("response = %+v, want successful ping", response)
	}
}

func TestServerLoggerCapsFileSize(t *testing.T) {
	logger := &serverLogger{path: filepath.Join(t.TempDir(), "server.log")}
	logger.writef("%s", strings.Repeat("x", maxServerLogSize))
	logger.writef("latest event")

	data, err := os.ReadFile(logger.path)
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

	server := &rotariServer{stopped: make(chan struct{}), lastAccess: time.Now()}
	go server.handle(t.TempDir(), serverConn)

	if _, err := client.Write([]byte("{invalid}\n")); err != nil {
		t.Fatal(err)
	}
	var response serverResponse
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

	server := &rotariServer{stopped: make(chan struct{}), lastAccess: time.Now()}
	go server.handle(t.TempDir(), serverConn)
	if err := json.NewEncoder(client).Encode(serverRequest{Op: "unknown"}); err != nil {
		t.Fatal(err)
	}
	var response serverResponse
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Message != "unknown server operation: unknown" {
		t.Fatalf("response = %+v, want unknown-operation error", response)
	}
}

func TestServerHandleSubmitPersistsQueue(t *testing.T) {
	baseDir := t.TempDir()
	client, serverConn := net.Pipe()
	defer client.Close()

	server := &rotariServer{stopped: make(chan struct{}), lastAccess: time.Now()}
	go server.handle(baseDir, serverConn)
	request := serverRequest{
		Op: "submit", QueueName: "demo", Command: []string{"printf", "hello"},
		JobName: "greeting", DependsOn: []string{"setup"},
	}
	if err := json.NewEncoder(client).Encode(request); err != nil {
		t.Fatal(err)
	}
	var response serverResponse
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("response = %+v, want successful submit", response)
	}

	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "greeting" || queue.Commands[0].ID == "" {
		t.Fatalf("queue = %+v, want one persisted named command with an ID", queue)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("meta phase = %q, want collecting", meta.Phase)
	}
}

func TestSendServerRequestOverUnixSocket(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-server-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })

	listener, err := net.Listen("unix", serverSocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &rotariServer{listener: listener, stopped: make(chan struct{}), lastAccess: time.Now()}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			server.handle(baseDir, conn)
		}
	}()

	response, err := sendServerRequest(baseDir, serverRequest{Op: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Protocol != serverProtocolVersion {
		t.Fatalf("response = %+v, want successful ping", response)
	}
}

func TestEnsureServerReusesCompatibleServer(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-existing-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })
	listener, err := net.Listen("unix", serverSocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &rotariServer{listener: listener, stopped: make(chan struct{}), lastAccess: time.Now()}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			server.handle(baseDir, conn)
		}
	}()

	if err := ensureServer(baseDir); err != nil {
		t.Fatalf("ensureServer returned error for compatible server: %v", err)
	}
}

// TestCmdCancelRejectsWholeRunFromWrongHostViaCLI exercises the actual CLI
// entry point (cmdCancel -> ensureServer -> sendServerRequest -> server.handle)
// rather than calling cancelQueueJobs directly, so the host-mismatch guard is
// verified on the same code path a real `rotari cancel` invocation uses.
func TestCmdCancelRejectsWholeRunFromWrongHostViaCLI(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-cli-host-mismatch-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })

	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	// A PID that isn't this test process's own, recorded as owned by
	// another host, mirrors a runner that is genuinely still active
	// elsewhere over a shared base directory.
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid() + 1, RunID: "run-1", StartedAt: nowRFC3339(), Host: "other-host"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("unix", serverSocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &rotariServer{listener: listener, stopped: make(chan struct{}), lastAccess: time.Now()}
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go server.handle(baseDir, conn)
		}
	}()

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

func TestCmdServerStatusReportsRunningServer(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-status-server-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(baseDir) })
	listener, err := net.Listen("unix", serverSocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &rotariServer{listener: listener, stopped: make(chan struct{}), lastAccess: time.Now()}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			server.handle(baseDir, conn)
		}
	}()

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
	for _, want := range []string{"server is running", baseDir} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("server status does not contain %q: %s", want, output)
		}
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
	listener, err := net.Listen("unix", serverSocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &rotariServer{listener: listener, stopped: make(chan struct{}), lastAccess: time.Now()}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			server.handle(baseDir, conn)
		}
	}()
	if err := registerServer(masterDir, serverRecord{BaseDir: baseDir, PID: os.Getpid(), LastSeen: nowRFC3339()}); err != nil {
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
	for _, want := range []string{"master=" + masterDir, "PID", baseDir} {
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
	listener, err := net.Listen("unix", serverSocketPath(liveBaseDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := &rotariServer{listener: listener, stopped: make(chan struct{}), lastAccess: time.Now()}
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			server.handle(liveBaseDir, conn)
		}
	}()

	live := serverRecord{BaseDir: liveBaseDir, PID: os.Getpid(), StartedAt: nowRFC3339(), LastSeen: nowRFC3339()}
	if err := registerServer(masterDir, live); err != nil {
		t.Fatal(err)
	}
	staleBaseDir := filepath.Join(t.TempDir(), "stale")
	if err := registerServer(masterDir, serverRecord{BaseDir: staleBaseDir, PID: 999999}); err != nil {
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
	if len(servers) != 1 || servers[0].BaseDir != liveBaseDir {
		t.Fatalf("servers = %+v, want only %q", servers, liveBaseDir)
	}
	for _, path := range []string{serverRecordPath(masterDir, staleBaseDir), malformedPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid record %q was not removed: %v", path, err)
		}
	}
}

func TestServerRegistryRecordLifecycle(t *testing.T) {
	masterDir := t.TempDir()
	baseDir := t.TempDir()
	record := serverRecord{
		BaseDir: baseDir, PID: 1234, StartedAt: "2026-01-01T00:00:00Z", LastSeen: "2026-01-01T00:00:00Z",
	}
	if err := registerServer(masterDir, record); err != nil {
		t.Fatal(err)
	}
	if err := touchServerRecord(masterDir, baseDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(serverRecordPath(masterDir, baseDir))
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
	for _, want := range []string{"PID", "LAST_SEEN", baseDir, "1234"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted server list does not contain %q:\n%s", want, formatted)
		}
	}
	if got := formatServerList(nil); got != "no running servers" {
		t.Fatalf("empty server list = %q", got)
	}
	if err := unregisterServer(masterDir, baseDir); err != nil {
		t.Fatal(err)
	}
	if err := unregisterServer(masterDir, baseDir); err != nil {
		t.Fatalf("second unregister failed: %v", err)
	}
}

func TestRunServerLifecycle(t *testing.T) {
	baseDir := t.TempDir()
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	result := make(chan int, 1)
	go func() {
		result <- runServer(baseDir)
	}()

	deadline := time.Now().Add(3 * time.Second)
	var response serverResponse
	var err error
	for time.Now().Before(deadline) {
		response, err = sendServerRequest(baseDir, serverRequest{Op: "ping"})
		if err == nil && response.OK {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil || !response.OK {
		t.Fatalf("server did not become ready: response=%+v err=%v", response, err)
	}
	if _, err := os.Stat(serverRecordPath(masterDir, baseDir)); err != nil {
		t.Fatalf("server registry record missing: %v", err)
	}

	response, err = sendServerRequest(baseDir, serverRequest{Op: "shutdown"})
	if err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("shutdown response = %+v", response)
	}
	select {
	case code := <-result:
		if code != 0 {
			t.Fatalf("runServer exit code = %d, want 0", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop after shutdown")
	}
	for _, path := range []string{
		serverSocketPath(baseDir), serverPIDPath(baseDir), serverRecordPath(masterDir, baseDir),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("server artifact %q remains after shutdown: %v", path, err)
		}
	}
}

func TestRunServerUsesOwnerOnlyPermissions(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-perm-server-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	result := make(chan int, 1)
	go func() {
		result <- runServer(baseDir)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, err = sendServerRequest(baseDir, serverRequest{Op: "ping"})
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}

	info, statErr := os.Stat(serverSocketPath(baseDir))
	if statErr != nil {
		t.Fatal(statErr)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("server socket permissions = %o, want no group/other access", perm)
	}

	if _, err := sendServerRequest(baseDir, serverRequest{Op: "shutdown"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop after shutdown")
	}
}
