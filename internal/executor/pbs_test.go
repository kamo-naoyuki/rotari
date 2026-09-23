package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSubmitPBSJobWithFakePBS(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "qsub", `#!/bin/sh
printf '123.headnode\n'
`)

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	runDir := t.TempDir()
	job := model.JobSpec{ID: "abc123", Command: []string{"echo", "hello"}}
	metadata, err := submitPBSJob(testStore(), testLogf, runDir, job, []string{"-l select=1:ncpus=2"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.PBSJobID != "123.headnode" {
		t.Fatalf("job id = %q, want 123.headnode", metadata.PBSJobID)
	}
	wrapper, err := os.ReadFile(filepath.Join(runDir, "abc123", "pbs-wrapper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "echo") {
		t.Fatalf("wrapper does not contain command: %s", wrapper)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "abc123", "job.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["executor"] != "pbs" {
		t.Fatalf("job metadata executor = %v, want pbs", raw["executor"])
	}
	if _, ok := raw["executors"]; ok {
		t.Fatal("job metadata uses obsolete executors key")
	}
}

func TestSubmitPBSArrayWithFakePBS(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "qsub-array-args")
	writeExecutable(t, binDir, "qsub", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
printf '123[].server\n'
`, argumentsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runDir := t.TempDir()
	taskOne, taskTwo := 1, 2
	jobs := []model.JobSpec{
		{ID: "array-1", ArrayGroup: "array", ArrayTaskID: &taskOne, ArrayFirst: 1, ArrayLast: 2, Command: []string{"echo", "hello"}, Environment: []string{"ROTARI_ARRAY_TASK_ID=1", "ROTARI_JOB_DIR=" + filepath.Join(runDir, "array-1")}},
		{ID: "array-2", ArrayGroup: "array", ArrayTaskID: &taskTwo, ArrayFirst: 1, ArrayLast: 2, Command: []string{"echo", "hello"}, Environment: []string{"ROTARI_ARRAY_TASK_ID=2", "ROTARI_JOB_DIR=" + filepath.Join(runDir, "array-2")}},
	}
	handles, err := submitPBSArray(testStore(), runDir, jobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(handles) != 2 || handles[0].Native != "123.server[1]" || handles[1].Native != "123.server[2]" {
		t.Fatalf("handles = %#v", handles)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "-J\n1-2\n") {
		t.Fatalf("qsub arguments = %q", arguments)
	}
	if !strings.Contains(string(arguments), "-o\n/dev/null\n") {
		t.Fatalf("qsub arguments = %q, want PBS output files disabled", arguments)
	}
	wrapper, err := os.ReadFile(filepath.Join(runDir, "array-pbs-array-wrapper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), `case "$PBS_ARRAY_INDEX"`) {
		t.Fatalf("PBS wrapper missing array variable: %s", wrapper)
	}
}

func TestPBSStatusCommandsWithFakePBS(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "qstat", `#!/bin/sh
if [ "$1" = "-xf" ]; then
    printf '    exit_status = 1\n'
elif [ "$1" = "-f" ]; then
	printf '    job_state = Q\n'
else
    exit 1
fi
`)
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	active, err := pbsJobActive("123.headnode")
	if err != nil || !active {
		t.Fatalf("pbsJobActive = %v, %v; want true, nil", active, err)
	}
	state, err := pbsJobState("123.headnode")
	if err != nil || state != "pending" {
		t.Fatalf("pbsJobState = %q, %v; want pending, nil", state, err)
	}
	exitCode, ok := pbsAccounting("123.headnode")
	if !ok || exitCode != 1 {
		t.Fatalf("pbsAccounting = %d, %v; want 1, true", exitCode, ok)
	}
}

func TestPBSExecutorSuspendsAndResumesJob(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "qsig-args")
	writeExecutable(t, binDir, "qsig", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" >> %q\n", argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	jobDir := t.TempDir()
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), pbsJobMetadata{Executor: "pbs", PBSJobID: "123.headnode"}); err != nil {
		t.Fatal(err)
	}
	pbs := PBS{Store: testStore()}
	if err := pbs.Suspend(jobDir); err != nil {
		t.Fatal(err)
	}
	if err := pbs.Resume(jobDir); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(args) != "-s\nsuspend\n123.headnode\n-s\nresume\n123.headnode\n" {
		t.Fatalf("qsig arguments = %q", args)
	}
}

func TestPBSReportsMissingSchedulerBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	jobDir := t.TempDir()
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), pbsJobMetadata{Executor: "pbs", PBSJobID: "123.headnode"}); err != nil {
		t.Fatal(err)
	}
	pbs := PBS{Store: testStore()}
	if err := pbs.Suspend(jobDir); err == nil || !strings.Contains(err.Error(), "not installed on this host") {
		t.Fatalf("Suspend error = %v, want a hint that qsig is missing on this host", err)
	}
	if err := pbs.Cancel(jobDir); err == nil || !strings.Contains(err.Error(), "not installed on this host") {
		t.Fatalf("Cancel error = %v, want a hint that qdel is missing on this host", err)
	}
}

func TestWaitPBSJobUsesWrapperStatus(t *testing.T) {
	store := testStore()
	runDir := t.TempDir()
	job := pbsJobMetadata{Executor: "pbs", JobID: "job-1", Command: []string{"echo", "hi"}, PBSJobID: "123.headnode"}
	jobDir := filepath.Join(runDir, job.JobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "status.json"), WrapperStatus{Phase: "finished", ExitCode: 7, Error: "failed"}); err != nil {
		t.Fatal(err)
	}

	result := waitPBSJob(store, runDir, job)
	if result.ExitCode != 7 || result.Error != "failed" {
		t.Fatalf("result = %+v, want exit 7 and failed", result)
	}
}
