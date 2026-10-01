package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSubmitSGEJobWithFakeGridEngine(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "qsub-args")
	writeExecutable(t, binDir, "qsub", fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nprintf '12345\\n'\n", argumentsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	runDir := t.TempDir()
	job := model.JobSpec{ID: "job-1", Command: []string{"echo", "hello"}, LogMode: model.LogModeSeparate}
	policy := sgeSubmissionPolicy{retry: schedulerSubmissionRetryPolicy{}, spacing: newSchedulerSubmissionGate(0)}
	metadata, err := submitSGEJobWithPolicies(testStore(), testLogf, runDir, job, []string{"-q short"}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.SGEJobID != "12345" || metadata.Executor != "sge" {
		t.Fatalf("metadata = %#v", metadata)
	}
	args, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-terse", "-o\n" + filepath.Join(runDir, "job-1", state.StdoutFileName), "-e\n" + filepath.Join(runDir, "job-1", state.StderrFileName), "-q\nshort", "-V"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("qsub args %q do not include %q", args, want)
		}
	}
	if _, err := os.Stat(filepath.Join(runDir, "job-1", "sge-wrapper.sh")); err != nil {
		t.Fatalf("wrapper was not created: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "job-1", "job.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded sgeJobMetadata
	if err := json.Unmarshal(data, &recorded); err != nil || recorded.SGEJobID != "12345" || recorded.Executor != "sge" {
		t.Fatalf("recorded job metadata = %#v, err=%v", recorded, err)
	}

	job.ID = "no-env"
	job.EnvMode = model.EnvModeNone
	if _, err := submitSGEJobWithPolicies(testStore(), nil, runDir, job, nil, policy); err != nil {
		t.Fatal(err)
	}
	args, err = os.ReadFile(argumentsPath)
	if err != nil || strings.Contains(string(args), "-V\n") {
		t.Fatalf("NONE qsub args = %q, err=%v; want no -V", args, err)
	}
}

func TestParseSGEJobID(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"12345\n", "12345"},
		{"12345.1-10:1\n", "12345"},
	} {
		got, err := parseSGEJobID(test.input)
		if err != nil || got != test.want {
			t.Errorf("parseSGEJobID(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
	for _, input := range []string{"", "job-id", "Your job 123 has been submitted"} {
		if _, err := parseSGEJobID(input); err == nil {
			t.Errorf("parseSGEJobID(%q) succeeded, want error", input)
		}
	}
}

func TestSGEStatusAndAccountingWithFakeGridEngine(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "qstat", `#!/bin/sh
cat <<'XML'
<?xml version="1.0"?>
<job_info><queue_info><job_list state="r"><JB_job_number>12345</JB_job_number></job_list></queue_info></job_info>
XML
`)
	writeExecutable(t, binDir, "qacct", "#!/bin/sh\nprintf 'failed 0\\nexit_status 7\\n'\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if state, err := sgeJobState("12345"); err != nil || state != "running" {
		t.Fatalf("sgeJobState = %q, %v; want running", state, err)
	}
	if active, err := sgeJobActive("12345"); err != nil || !active {
		t.Fatalf("sgeJobActive = %v, %v; want true", active, err)
	}
	if code, ok, err := sgeAccounting("12345"); err != nil || !ok || code != 7 {
		t.Fatalf("sgeAccounting = %d, %v, %v; want 7, true, nil", code, ok, err)
	}
}

func TestSGEControlsUseRecordedJobID(t *testing.T) {
	binDir := t.TempDir()
	argsPath := filepath.Join(t.TempDir(), "control-args")
	writeExecutable(t, binDir, "qdel", fmt.Sprintf("#!/bin/sh\nprintf 'qdel %%s\\n' \"$*\" >> %q\n", argsPath))
	writeExecutable(t, binDir, "qmod", fmt.Sprintf("#!/bin/sh\nprintf 'qmod %%s\\n' \"$*\" >> %q\n", argsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	jobDir := t.TempDir()
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), sgeJobMetadata{Executor: "sge", SGEJobID: "12345"}); err != nil {
		t.Fatal(err)
	}
	sge := SGE{Store: testStore()}
	if err := sge.Suspend(jobDir); err != nil {
		t.Fatal(err)
	}
	if err := sge.Resume(jobDir); err != nil {
		t.Fatal(err)
	}
	if err := sge.Cancel(jobDir); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil || string(args) != "qmod -sj 12345\nqmod -usj 12345\nqdel 12345\n" {
		t.Fatalf("control args = %q, err=%v", args, err)
	}
}

func TestSGEWithRunSettingsCopiesSubmissionPolicy(t *testing.T) {
	base := NewSGE(testStore(), nil)
	configured, ok := base.WithRunSettings(RunSettings{SubmitInterval: 250 * time.Millisecond, SubmitRetryLimit: 4}).(SGE)
	if !ok || configured.SubmissionRetry.RetryLimit != 4 || configured.SubmissionInterval != 250*time.Millisecond {
		t.Fatalf("configured executor = %#v, want SGE with custom submit policy", configured)
	}
	if base.SubmissionInterval != 0 || base.SubmissionRetry.RetryLimit != schedulerSubmissionRetries.RetryLimit {
		t.Fatalf("base executor was mutated: %#v", base)
	}
}

func TestWaitSGEJobUsesWrapperStatus(t *testing.T) {
	store := testStore()
	runDir := t.TempDir()
	job := sgeJobMetadata{Executor: "sge", JobID: "job-1", Command: []string{"echo", "hi"}, SGEJobID: "12345"}
	jobDir := filepath.Join(runDir, job.JobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "status.json"), WrapperStatus{Phase: "finished", ExitCode: 9, Error: "failed"}); err != nil {
		t.Fatal(err)
	}
	result := waitSGEJob(store, runDir, job)
	if result.ExitCode != 9 || result.Error != "failed" {
		t.Fatalf("result = %+v, want wrapper exit 9", result)
	}
}
