package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
)

func TestSubmitLSFJobWithFakeLSF(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "bsub-args")
	writeExecutable(t, binDir, "bsub", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
cat >/dev/null
printf 'Job <123> is submitted to default queue.\n'
`, argumentsPath))
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	runDir := t.TempDir()
	job := model.JobSpec{ID: "abc123", AttemptID: "att_run-1-abc123-0", LogMode: model.LogModeSeparate, Command: []string{"echo", "hello"}}
	metadata, err := submitLSFJob(testStore(), testLogf, runDir, job, []string{"-q short", "-n 2"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.LSFJobID != "123" {
		t.Fatalf("job id = %q, want 123", metadata.LSFJobID)
	}
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		t.Fatal(err)
	}
	wrapper, err := os.ReadFile(filepath.Join(jobDir, "lsf-wrapper.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "#BSUB -o") || !strings.Contains(string(wrapper), "echo") {
		t.Fatalf("wrapper is missing LSF directives or command: %s", wrapper)
	}
	if !strings.Contains(string(wrapper), "#BSUB -o '"+filepath.Join(jobDir, state.StdoutFileName)+"'\n") || !strings.Contains(string(wrapper), "#BSUB -e '"+filepath.Join(jobDir, state.StderrFileName)+"'\n") {
		t.Fatalf("LSF wrapper does not separate stream files: %s", wrapper)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil || !strings.Contains(string(arguments), "-env\nall\n") || !strings.Contains(string(arguments), "-J\nrotari show att_run-1-abc123-0 (echo hello)\n") {
		t.Fatalf("ALL bsub arguments = %q, err=%v; want -env all", arguments, err)
	}
	job.ID = "none-job"
	job.EnvMode = model.EnvModeNone
	if _, err := submitLSFJob(testStore(), testLogf, runDir, job, nil); err != nil {
		t.Fatal(err)
	}
	arguments, err = os.ReadFile(argumentsPath)
	if err != nil || !strings.Contains(string(arguments), "-env\nnone\n") {
		t.Fatalf("NONE bsub arguments = %q, err=%v; want -env none", arguments, err)
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
	if !strings.Contains(string(arguments), "-env\nall\n") {
		t.Fatalf("bsub arguments = %q, want array default -env all", arguments)
	}
	// LSF array output is controlled by directives in the submitted wrapper.
	wrapper, err := os.ReadFile(wrapperPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "#BSUB -o /dev/null\n") || !strings.Contains(string(wrapper), "#BSUB -e /dev/null\n") {
		t.Fatalf("LSF array wrapper = %q, want scheduler output files disabled", wrapper)
	}
	jobs[0].EnvMode = model.EnvModeNone
	jobs[1].EnvMode = model.EnvModeNone
	if _, err := submitLSFArray(testStore(), testLogf, runDir, jobs, nil); err != nil {
		t.Fatal(err)
	}
	arguments, err = os.ReadFile(argumentsPath)
	if err != nil || !strings.Contains(string(arguments), "-env\nnone\n") {
		t.Fatalf("NONE array bsub arguments = %q, err=%v", arguments, err)
	}
	wrapper, err = os.ReadFile(wrapperPath)
	if err != nil || !strings.Contains(string(wrapper), "env -i ") {
		t.Fatalf("NONE array wrapper = %q, err=%v; want clean task environment", wrapper, err)
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
	exitCode, _, ok, err := lsfAccounting("123")
	if err != nil || !ok || exitCode != 0 {
		t.Fatalf("lsfAccounting = %d, %v, %v; want 0, true, nil", exitCode, ok, err)
	}
}

// bjobs keeps listing a finished job for CLEAN_PERIOD; its DONE or EXIT state
// must send rotari to accounting rather than count as a job still queued.
func TestLSFJobStateTreatsFinishedJobsAsEnded(t *testing.T) {
	for _, stat := range []string{"DONE", "EXIT"} {
		t.Run(stat, func(t *testing.T) {
			binDir := t.TempDir()
			writeExecutable(t, binDir, "bjobs", fmt.Sprintf("#!/bin/sh\nprintf '%s\\n'\n", stat))
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			state, err := lsfJobState("123")
			if err != nil || state != "" {
				t.Fatalf("lsfJobState = %q, %v; want no queue state", state, err)
			}
		})
	}
}

const lsfHistoryPending = `
Job <2>, User <rotari>, Project <default>, Command <#BSUB -o '/state/r179204861
                     8535565155/output';sh -c 'exit 1'>
Thu Oct 15 07:17:01: Submitted from host <lsfhost>, to Queue <normal>, CWD </>,
                      Output File </state/output>;


Summary of time in seconds spent in various states by  Thu Oct 15 07:17:02
  PEND     PSUSP    RUN      USUSP    SSUSP    UNKWN    TOTAL
  1        0        0        0        0        0        1
`

const lsfHistoryExited = `
Job <3>, User <rotari>, Project <default>, Command <echo DONE; exit 3>
Thu Oct 15 07:17:01: Submitted from host <lsfhost>, to Queue <normal>, CWD </>;
Thu Oct 15 07:17:02: Dispatched 1 Task(s) on Host(s) <lsfhost>, Allocated 1 Slot
                     (s) on Host(s) <lsfhost>, Effective RES_REQ <select[type
                     == any] order[r15s:pg] >;
Thu Oct 15 07:17:02: Starting (Pid 123);
Thu Oct 15 07:17:03: Exited with exit code 3. The CPU time used is 0.0 seconds;
Thu Oct 15 07:17:03: Completed <exit>;


Summary of time in seconds spent in various states by  Thu Oct 15 07:17:04
  PEND     PSUSP    RUN      USUSP    SSUSP    UNKWN    TOTAL
  1        0        1        0        0        0        2
`

const lsfHistoryDone = `
Job <4>, User <rotari>, Project <default>, Command <echo Exited with exit code 7>
Thu Oct 15 07:17:01: Submitted from host <lsfhost>, to Queue <normal>, CWD </>;
Thu Oct 15 07:17:02: Starting (Pid 123);
Thu Oct 15 07:17:03: Done successfully. The CPU time used is 0.0 seconds;
Thu Oct 15 07:17:03: Post job process done successfully;


Summary of time in seconds spent in various states by  Thu Oct 15 07:17:04
  PEND     PSUSP    RUN      USUSP    SSUSP    UNKWN    TOTAL
  1        0        1        0        0        0        2
`

const lsfHistoryKilled = `
Job <5>, User <rotari>, Project <default>, Command <sleep 300>
Thu Oct 15 07:17:01: Submitted from host <lsfhost>, to Queue <normal>, CWD </>;
Thu Oct 15 07:17:02: Signal <KILL> requested by user or administrator <rotari>;
Thu Oct 15 07:17:02: Exited by LSF signal TERM_OWNER. The CPU time used is 0.0 seconds;
Thu Oct 15 07:17:02: Completed <exit>; TERM_OWNER: job killed by owner;


Summary of time in seconds spent in various states by  Thu Oct 15 07:17:04
  PEND     PSUSP    RUN      USUSP    SSUSP    UNKWN    TOTAL
  1        0        0        0        0        0        1
`

// A job bjobs does not list yet or any more is read from bhist, whose time
// summary and command text hold numbers and words that are not its result.
func TestLSFAccountingReadsOnlyTheJobResult(t *testing.T) {
	notFound := "Job <2> is not found\n"
	for _, test := range []struct {
		name     string
		bjobs    string
		bjobsErr bool
		bhist    string
		code     int
		ok       bool
		state    string
	}{
		{name: "pending job missing from bjobs", bjobs: notFound, bjobsErr: true, bhist: lsfHistoryPending},
		{name: "bjobs pending", bjobs: "PEND -\n", bhist: lsfHistoryPending, state: "pending"},
		{name: "bjobs running", bjobs: "RUN -\n", bhist: lsfHistoryPending, state: "running"},
		{name: "bjobs done", bjobs: "DONE -\n", code: 0, ok: true},
		{name: "bjobs exit", bjobs: "EXIT 3\n", code: 3, ok: true},
		{name: "bjobs exit without code", bjobs: "EXIT -\n", code: 1, ok: true},
		{name: "bhist exited", bjobs: notFound, bjobsErr: true, bhist: lsfHistoryExited, code: 3, ok: true},
		{name: "bhist done", bjobs: notFound, bjobsErr: true, bhist: lsfHistoryDone, code: 0, ok: true},
		{name: "bhist killed", bjobs: notFound, bjobsErr: true, bhist: lsfHistoryKilled, code: 1, ok: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			binDir := t.TempDir()
			bjobsOutput := filepath.Join(binDir, "bjobs.out")
			bhistOutput := filepath.Join(binDir, "bhist.out")
			if err := os.WriteFile(bjobsOutput, []byte(test.bjobs), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bhistOutput, []byte(test.bhist), 0o600); err != nil {
				t.Fatal(err)
			}
			bjobsExit := 0
			if test.bjobsErr {
				bjobsExit = 255
			}
			writeExecutable(t, binDir, "bjobs", fmt.Sprintf("#!/bin/sh\ncat %q\nexit %d\n", bjobsOutput, bjobsExit))
			writeExecutable(t, binDir, "bhist", fmt.Sprintf("#!/bin/sh\ncat %q\n", bhistOutput))
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			code, state, ok, err := lsfAccounting("2")
			if err != nil || ok != test.ok || code != test.code || (!ok && state != test.state) {
				t.Fatalf("lsfAccounting = %d, %q, %v, %v; want %d, %q, %v, nil", code, state, ok, err, test.code, test.state, test.ok)
			}
		})
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
