package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSubmitLSFJobWithFakeLSF(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "bsub", `#!/bin/sh
cat >/dev/null
printf 'Job <123> is submitted to default queue.\n'
`)
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	runDir := t.TempDir()
	job := model.JobSpec{ID: "abc123", Command: []string{"echo", "hello"}}
	metadata, err := submitLSFJob(testStore(), testLogf, runDir, job, []string{"-q short", "-n 2"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.LSFJobID != "123" {
		t.Fatalf("job id = %q, want 123", metadata.LSFJobID)
	}
	wrapper, err := os.ReadFile(filepath.Join(runDir, "abc123", "lsf-wrapper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "#BSUB -o") || !strings.Contains(string(wrapper), "echo") {
		t.Fatalf("wrapper is missing LSF directives or command: %s", wrapper)
	}
}

func TestSubmitLSFJobFailureDoesNotRetryOrRecordJob(t *testing.T) {
	binDir := t.TempDir()
	callsPath := filepath.Join(t.TempDir(), "bsub-calls")
	writeExecutable(t, binDir, "bsub", fmt.Sprintf("#!/bin/sh\nprintf x >> %q\ncat >/dev/null\nprintf 'invalid queue\\n' >&2\nexit 1\n", callsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	runDir := t.TempDir()
	job := model.JobSpec{ID: "job-1", Command: []string{"echo", "hello"}}
	if _, err := submitLSFJob(testStore(), testLogf, runDir, job, nil); err == nil {
		t.Fatal("submitLSFJob succeeded, want scheduler error")
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil || string(calls) != "x" {
		t.Fatalf("bsub calls = %q, err=%v; want one call", calls, err)
	}
	if _, err := os.Stat(filepath.Join(runDir, job.ID, "job.json")); !os.IsNotExist(err) {
		t.Fatalf("job metadata exists after failed submission: %v", err)
	}
}

func TestSubmitLSFArrayWithFakeLSF(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "bsub-array-args")
	wrapperPath := filepath.Join(t.TempDir(), "bsub-array-wrapper")
	writeExecutable(t, binDir, "bsub", fmt.Sprintf(`#!/bin/sh
cat > %q
printf 'Job <123> is submitted to default queue.\n'
printf '%%s\n' "$@" > %q
`, wrapperPath, argumentsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runDir := t.TempDir()
	taskOne, taskTwo := 1, 2
	jobs := []model.JobSpec{
		{ID: "array-1", ArrayGroup: "array", ArrayTaskID: &taskOne, ArrayFirst: 1, ArrayLast: 2, Command: []string{"echo", "hello"}, Environment: []string{"ROTARI_ARRAY_TASK_ID=1", "ROTARI_JOB_DIR=" + filepath.Join(runDir, "array-1")}},
		{ID: "array-2", ArrayGroup: "array", ArrayTaskID: &taskTwo, ArrayFirst: 1, ArrayLast: 2, Command: []string{"echo", "hello"}, Environment: []string{"ROTARI_ARRAY_TASK_ID=2", "ROTARI_JOB_DIR=" + filepath.Join(runDir, "array-2")}},
	}
	handles, err := submitLSFArray(testStore(), testLogf, runDir, jobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(handles) != 2 || handles[0].Native != "123[1]" || handles[1].Native != "123[2]" {
		t.Fatalf("handles = %#v", handles)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "-J\nrotari[1-2]\n") {
		t.Fatalf("bsub arguments = %q", arguments)
	}
	// LSF array output is controlled by directives in the submitted wrapper.
	wrapper, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "#BSUB -o /dev/null\n") || !strings.Contains(string(wrapper), "#BSUB -e /dev/null\n") {
		t.Fatalf("LSF array wrapper = %q, want scheduler output files disabled", wrapper)
	}
}

func TestLSFStatusCommandsWithFakeLSF(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "bjobs", `#!/bin/sh
if [ "$1" = "-a" ]; then
    printf 'DONE 0\n'
    exit 0
fi
printf 'PEND\n'
`)
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	active, err := lsfJobActive("123")
	if err != nil || !active {
		t.Fatalf("lsfJobActive = %v, %v; want true, nil", active, err)
	}
	state, err := lsfJobState("123")
	if err != nil || state != "pending" {
		t.Fatalf("lsfJobState = %q, %v; want pending, nil", state, err)
	}
	exitCode, ok, err := lsfAccounting("123")
	if err != nil || !ok || exitCode != 0 {
		t.Fatalf("lsfAccounting = %d, %v, %v; want 0, true, nil", exitCode, ok, err)
	}
}

func TestLSFExecutorSuspendsAndResumesJob(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "control-args")
	writeExecutable(t, binDir, "bstop", fmt.Sprintf("#!/bin/sh\nprintf 'bstop %%s\\n' \"$@\" >> %q\n", argumentsPath))
	writeExecutable(t, binDir, "bresume", fmt.Sprintf("#!/bin/sh\nprintf 'bresume %%s\\n' \"$@\" >> %q\n", argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	jobDir := t.TempDir()
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), lsfJobMetadata{Executor: "lsf", LSFJobID: "123"}); err != nil {
		t.Fatal(err)
	}
	lsf := LSF{Store: testStore()}
	if err := lsf.Suspend(jobDir); err != nil {
		t.Fatal(err)
	}
	if err := lsf.Resume(jobDir); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "bstop 123\nbresume 123\n" {
		t.Fatalf("control arguments = %q", args)
	}
}

func TestLSFReportsMissingSchedulerBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	jobDir := t.TempDir()
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), lsfJobMetadata{Executor: "lsf", LSFJobID: "123"}); err != nil {
		t.Fatal(err)
	}
	lsf := LSF{Store: testStore()}
	if err := lsf.Suspend(jobDir); err == nil || !strings.Contains(err.Error(), "not installed on this host") {
		t.Fatalf("Suspend error = %v, want a hint that bstop is missing on this host", err)
	}
	if err := lsf.Cancel(jobDir); err == nil || !strings.Contains(err.Error(), "not installed on this host") {
		t.Fatalf("Cancel error = %v, want a hint that bkill is missing on this host", err)
	}
}

func TestWaitLSFJobUsesWrapperStatus(t *testing.T) {
	store := testStore()
	runDir := t.TempDir()
	job := lsfJobMetadata{Executor: "lsf", JobID: "job-1", Command: []string{"echo", "hi"}, LSFJobID: "123"}
	jobDir := filepath.Join(runDir, job.JobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "status.json"), WrapperStatus{Phase: "finished", ExitCode: 9, Error: "failed"}); err != nil {
		t.Fatal(err)
	}

	result := waitLSFJob(store, runDir, job)
	if result.ExitCode != 9 || result.Error != "failed" {
		t.Fatalf("result = %+v, want exit 9 and failed", result)
	}
}

func TestWaitLSFJobPersistsNormalizedSchedulerStatus(t *testing.T) {
	store := testStore()
	binDir := t.TempDir()
	runDir := t.TempDir()
	job := lsfJobMetadata{Executor: "lsf", JobID: "job-1", Command: []string{"echo", "hi"}, LSFJobID: "123"}
	jobDir := filepath.Join(runDir, job.JobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(jobDir, "status.json")
	writeExecutable(t, binDir, "bjobs", fmt.Sprintf(`#!/bin/sh
printf '{"phase":"finished","exit_code":0}\n' > %q
printf 'RUN\n'
`, statusPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	result := waitLSFJob(store, runDir, job)
	if result.ExitCode != 0 || result.Error != "" {
		t.Fatalf("result = %+v, want successful result", result)
	}
	if got := LoadSchedulerStatus(store, jobDir); got != "running" {
		t.Fatalf("scheduler state = %q, want running", got)
	}
}
