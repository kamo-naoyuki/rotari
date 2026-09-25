package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSubmitSlurmJobWithFakeSlurm(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "sbatch-args")
	writeExecutable(t, binDir, "sbatch", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
printf '12345;fake-host\n'
`, argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	baseDir := t.TempDir()
	runDir := filepath.Join(baseDir, "projects", "demo", "runs", "run-1")
	job := model.JobSpec{ID: "abc123", AttemptID: "att_run-1-abc123-0", Command: []string{"echo", "hello"}}
	metadata, err := submitSlurmJob(testStore(), testLogf, runDir, job, []string{"-p short --cpus-per-task=2"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.SlurmJobID != "12345" {
		t.Fatalf("job id = %q, want 12345", metadata.SlurmJobID)
	}
	if metadata.SubmittedAt == "" {
		t.Fatal("submitted_at is empty")
	}
	if !strings.Contains(metadata.Command[0], "echo") {
		t.Fatalf("unexpected command: %#v", metadata.Command)
	}
	wrapper, err := os.ReadFile(filepath.Join(runDir, "abc123", "attempts", "att_run-1-abc123-0", "slurm-wrapper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "echo") {
		t.Fatalf("wrapper does not contain command: %s", wrapper)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	wantShowCommand := "--job-name=rotari show --job-id 'att_run-1-abc123-0'"
	if !strings.Contains(string(arguments), wantShowCommand+"\n") {
		t.Fatalf("sbatch arguments = %q, want %q", arguments, wantShowCommand)
	}
}

func TestSubmitSlurmArrayWithFakeSlurm(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "sbatch-array-args")
	writeExecutable(t, binDir, "sbatch", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
printf '54321;fake-host\n'
`, argumentsPath))
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	runDir := filepath.Join(t.TempDir(), "runs", "run-1")
	taskOne, taskTwo := 1, 2
	jobs := []model.JobSpec{
		{ID: "array-1", ArrayGroup: "array", ArrayTaskID: &taskOne, ArrayFirst: 1, ArrayLast: 2, Command: []string{"echo", "hello"}, Environment: []string{"ROTARI_ARRAY_TASK_ID=1", "ROTARI_JOB_DIR=" + filepath.Join(runDir, "array-1")}},
		{ID: "array-2", ArrayGroup: "array", ArrayTaskID: &taskTwo, ArrayFirst: 1, ArrayLast: 2, Command: []string{"echo", "hello"}, Environment: []string{"ROTARI_ARRAY_TASK_ID=2", "ROTARI_JOB_DIR=" + filepath.Join(runDir, "array-2")}},
	}
	handles, err := submitSlurmArray(testStore(), runDir, jobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(handles) != 2 || handles[0].Native != "54321_1" || handles[1].Native != "54321_2" {
		t.Fatalf("handles = %#v", handles)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "--array=1-2\n") {
		t.Fatalf("sbatch arguments = %q, want native array range", arguments)
	}
	if !strings.Contains(string(arguments), "--output=/dev/null\n") || !strings.Contains(string(arguments), "--error=/dev/null\n") {
		t.Fatalf("sbatch arguments = %q, want Slurm output files disabled", arguments)
	}
	wrapper, err := os.ReadFile(filepath.Join(runDir, "array-array-wrapper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "ROTARI_ARRAY_TASK_ID='1'") || !strings.Contains(string(wrapper), "job_dir='"+filepath.Join(runDir, "array-1")+"'") || strings.Contains(string(wrapper), `job_dir="$ROTARI_JOB_DIR"`) {
		t.Fatalf("array wrapper missing task environment: %s", wrapper)
	}
}

func TestSubmitSlurmSparseArrayWithFakeSlurm(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "sbatch-sparse-array-args")
	writeExecutable(t, binDir, "sbatch", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
printf '54321;fake-host\n'
`, argumentsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	runDir := filepath.Join(t.TempDir(), "runs", "run-1")
	taskOne, taskThree, taskFour := 1, 3, 4
	jobs := []model.JobSpec{
		{ID: "array-1", ArrayGroup: "array", ArrayTaskID: &taskOne, ArrayFirst: 1, ArrayLast: 4, Command: []string{"echo", "hello"}},
		{ID: "array-3", ArrayGroup: "array", ArrayTaskID: &taskThree, ArrayFirst: 1, ArrayLast: 4, Command: []string{"echo", "hello"}},
		{ID: "array-4", ArrayGroup: "array", ArrayTaskID: &taskFour, ArrayFirst: 1, ArrayLast: 4, Command: []string{"echo", "hello"}},
	}
	if _, err := submitSlurmArray(testStore(), runDir, jobs, nil); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "--array=1,3,4\n") {
		t.Fatalf("sbatch arguments = %q, want sparse native array", arguments)
	}
}

func TestSlurmStatusCommandsWithFakeSlurm(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "squeue", "#!/bin/sh\nprintf 'PENDING\\n'\n")
	writeExecutable(t, binDir, "sacct", "#!/bin/sh\nprintf 'FAILED|1:0\\n'\n")
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	active, err := slurmJobActive("12345")
	if err != nil || !active {
		t.Fatalf("slurmJobActive = %v, %v; want true, nil", active, err)
	}
	state, err := slurmJobState("12345")
	if err != nil || state != "pending" {
		t.Fatalf("slurmJobState = %q, %v; want pending, nil", state, err)
	}
	exitCode, state, ok := slurmAccounting("12345")
	if !ok || exitCode != 1 || state != "failed" {
		t.Fatalf("slurmAccounting = %d, %q, %v; want 1, failed, true", exitCode, state, ok)
	}
}

func TestWaitSlurmJobUsesWrapperStatus(t *testing.T) {
	store := testStore()
	runDir := t.TempDir()
	job := slurmJobMetadata{Executor: "slurm", JobID: "job-1", Command: []string{"echo", "hi"}, SlurmJobID: "12345"}
	jobDir := filepath.Join(runDir, job.JobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "status.json"), WrapperStatus{Phase: "finished", ExitCode: 8, Error: "failed", Hosts: []string{"compute-01", "compute-02"}}); err != nil {
		t.Fatal(err)
	}

	result := waitSlurmJob(store, runDir, job)
	if result.ExitCode != 8 || result.Error != "failed" || len(result.Hosts) != 2 || result.Hosts[1] != "compute-02" {
		t.Fatalf("result = %+v, want exit 8 and failed", result)
	}
}

func TestWaitSlurmJobFailureBoundariesWithFakeSlurm(t *testing.T) {
	store := testStore()
	oldAccountingWait := slurmAccountingWait
	oldPollInterval := slurmPollInterval
	slurmAccountingWait = 20 * time.Millisecond
	slurmPollInterval = time.Millisecond
	t.Cleanup(func() {
		slurmAccountingWait = oldAccountingWait
		slurmPollInterval = oldPollInterval
	})

	for _, testCase := range []struct {
		name       string
		statusJSON string
		squeue     string
		squeueCode int
		sacct      string
		sacctCode  int
		wantCode   int
		wantError  string
	}{
		{
			name:      "accounting and wrapper unavailable",
			squeue:    "",
			sacctCode: 1,
			wantCode:  1,
			wantError: "Slurm accounting result and wrapper status are unavailable",
		},
		{
			name:       "squeue fails like a down slurmctld falls back to accounting",
			squeueCode: 1,
			sacct:      "FAILED|7:0\n",
			wantCode:   7,
			wantError:  "failed",
		},
		{
			name:       "corrupted wrapper status falls back to accounting",
			statusJSON: "{not-json",
			squeue:     "",
			sacct:      "FAILED|7:0\n",
			wantCode:   7,
			wantError:  "failed",
		},
		{
			name:       "scheduler gone but wrapper finished",
			statusJSON: `{"phase":"finished","exit_code":3,"error":"wrapper-failed"}`,
			squeue:     "",
			sacctCode:  1,
			wantCode:   3,
			wantError:  "wrapper-failed",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			binDir := t.TempDir()
			if testCase.squeueCode == 0 {
				writeExecutable(t, binDir, "squeue", fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q\n", testCase.squeue))
			} else {
				writeExecutable(t, binDir, "squeue", fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q\nexit %d\n", testCase.squeue, testCase.squeueCode))
			}
			if testCase.sacctCode == 0 {
				writeExecutable(t, binDir, "sacct", fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q\n", testCase.sacct))
			} else {
				writeExecutable(t, binDir, "sacct", fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q\nexit %d\n", testCase.sacct, testCase.sacctCode))
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			runDir := t.TempDir()
			job := slurmJobMetadata{Executor: "slurm", JobID: "job-1", Command: []string{"echo", "hi"}, SlurmJobID: "12345"}
			jobDir := filepath.Join(runDir, job.JobID)
			if err := os.MkdirAll(jobDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if testCase.statusJSON != "" {
				if err := os.WriteFile(filepath.Join(jobDir, "status.json"), []byte(testCase.statusJSON), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			result := waitSlurmJob(store, runDir, job)
			if result.ExitCode != testCase.wantCode || result.Error != testCase.wantError {
				t.Fatalf("result = %+v, want exit %d error %q", result, testCase.wantCode, testCase.wantError)
			}
		})
	}
}

func TestWaitSlurmJobUsesWrapperStatusAfterSchedulerStillRunning(t *testing.T) {
	store := testStore()
	oldAccountingWait := slurmAccountingWait
	oldPollInterval := slurmPollInterval
	slurmAccountingWait = 20 * time.Millisecond
	slurmPollInterval = time.Millisecond
	t.Cleanup(func() {
		slurmAccountingWait = oldAccountingWait
		slurmPollInterval = oldPollInterval
	})

	runDir := t.TempDir()
	job := slurmJobMetadata{Executor: "slurm", JobID: "job-1", Command: []string{"echo", "hi"}, SlurmJobID: "12345"}
	jobDir := filepath.Join(runDir, job.JobID)
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(jobDir, "status.json")

	binDir := t.TempDir()
	writeExecutable(t, binDir, "squeue", fmt.Sprintf(`#!/bin/sh
printf 'RUNNING\n'
cat > %q <<'JSON'
{"phase":"finished","exit_code":4,"error":"wrapper-finished"}
JSON
`, statusPath))
	writeExecutable(t, binDir, "sacct", "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result := waitSlurmJob(store, runDir, job)
	if result.ExitCode != 4 || result.Error != "wrapper-finished" {
		t.Fatalf("result = %+v, want wrapper status after scheduler running", result)
	}
	if got := LoadSchedulerStatus(store, jobDir); got != "running" {
		t.Fatalf("scheduler status = %q, want running", got)
	}
}

func TestParseSlurmExitCode(t *testing.T) {
	cases := map[string]int{"0:0": 0, "1:0": 1, "2:15": 2, "invalid": 1}
	for input, want := range cases {
		if got := parseSlurmExitCode(input); got != want {
			t.Errorf("parseSlurmExitCode(%q) = %d, want %d", input, got, want)
		}
	}
}
