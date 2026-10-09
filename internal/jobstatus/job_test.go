package jobstatus

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestResolveJobPrefersAttemptOverSummary(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status"), "3\n")
	summary := model.JobResult{ID: "job-1", AttemptID: "att-1", ExitCode: 0, Hosts: []string{"summary-host"}}
	job := ReadJob(testStore(), jobDir, summary, true)
	if job.Source != SourceStatus || job.ExitCode != 3 || job.Blocked() {
		t.Fatalf("ReadJob() = %#v, want status file exit code", job)
	}
	result, ok := job.Result(model.JobSpec{ID: "job-1"})
	if !ok || result.ExitCode != 3 || result.AttemptID != "att-1" || !reflect.DeepEqual(result.Hosts, []string{"summary-host"}) {
		t.Fatalf("Result() = %#v, %v, want summary metadata with attempt exit code", result, ok)
	}
}

func TestResolveJobFallsBackToSummary(t *testing.T) {
	jobDir := filepath.Join(t.TempDir(), "missing")
	tests := []struct {
		name         string
		summary      model.JobResult
		wantBlocked  bool
		wantAccepted bool
	}{
		{name: "failure", summary: model.JobResult{ID: "job-1", ExitCode: 1, Error: "submit failed"}},
		{name: "blocked", summary: model.JobResult{ID: "job-1", ExitCode: 1, Error: "blocked by failed dependency"}, wantBlocked: true},
		{name: "accepted", summary: model.JobResult{ID: "job-1", ExitCode: 0, Accepted: true}, wantAccepted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := ReadJob(testStore(), jobDir, test.summary, true)
			if job.Source != SourceSummary || job.ExitCode != test.summary.ExitCode {
				t.Fatalf("ReadJob() = %#v, want summary fallback", job)
			}
			if job.Blocked() != test.wantBlocked || job.Accepted() != test.wantAccepted {
				t.Fatalf("Blocked() = %v, Accepted() = %v", job.Blocked(), job.Accepted())
			}
			if result, ok := job.Result(model.JobSpec{ID: "job-1"}); !ok || !reflect.DeepEqual(result, test.summary) {
				t.Fatalf("Result() = %#v, %v, want summary result", result, ok)
			}
		})
	}
}

func TestResolveJobBlockedOnlyFromSummary(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status"), "1\n")
	job := ReadJob(testStore(), jobDir, model.JobResult{ID: "job-1", ExitCode: 1, Error: "blocked by failed dependency"}, true)
	if job.Blocked() {
		t.Fatal("a job with its own status was reported as blocked")
	}
}

func TestResolveJobWithoutOutcome(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status.json"), `{"phase":"running","hosts":["node1"]}`)
	job := ReadJob(testStore(), jobDir, model.JobResult{}, false)
	if job.Finished() {
		t.Fatalf("ReadJob() = %#v, want unfinished job", job)
	}
	if _, ok := job.Result(model.JobSpec{ID: "job-1"}); ok {
		t.Fatal("unfinished job returned a result")
	}
	if !reflect.DeepEqual(job.Hosts(), []string{"node1"}) {
		t.Fatalf("Hosts() = %#v, want wrapper hosts", job.Hosts())
	}
}

func TestDisplayStatusUsesPersistedAttemptAndResultFacts(t *testing.T) {
	tests := []struct {
		name string
		job  Job
		spec model.JobSpec
		want string
	}{
		{name: "no persisted evidence", want: "unknown"},
		{name: "attempt with no phase", job: Job{Attempt: Attempt{HasAttempt: true}}, want: "unknown"},
		{name: "wrapper running", job: Job{Attempt: Attempt{HasWrapper: true, Wrapper: executor.WrapperStatus{Phase: "running"}}}, want: "running (recorded)"},
		{name: "scheduler pending", job: Job{Attempt: Attempt{SchedulerState: "pending"}}, want: "waiting (recorded)"},
		{name: "suspended", job: Job{Attempt: Attempt{SchedulerState: "suspended"}}, want: "suspended (recorded)"},
		{name: "success", job: Job{ExitCode: 0, Source: SourceStatus}, want: model.StatusSuccess},
		{name: "failure", job: Job{ExitCode: 7, Source: SourceStatus}, want: model.StatusFailed},
		{name: "cancelled wrapper", job: Job{ExitCode: 143, Source: SourceWrapper, Attempt: Attempt{HasWrapper: true, Wrapper: executor.WrapperStatus{Phase: "cancelled"}}}, want: model.StatusCancelled},
		{name: "blocked", job: Job{ExitCode: 1, Source: SourceSummary, Summary: model.JobResult{Error: "blocked by dependency"}, HasSummary: true}, want: "blocked"},
		{name: "unknown scheduler", job: Job{ExitCode: 1, Source: SourceScheduler, Attempt: Attempt{SchedulerState: "unknown"}}, want: "unknown"},
		{name: "assigned but missing attempt", job: Job{}, spec: model.JobSpec{AttemptID: "att_run-job-0"}, want: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.job.DisplayStatus(test.spec); got != test.want {
				t.Fatalf("DisplayStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadJobDisplayStatusWithoutUsableMetadata(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "malformed", "inaccessible", "unrecognized"} {
		t.Run(kind, func(t *testing.T) {
			jobDir := filepath.Join(t.TempDir(), "run", "job")
			if kind != "missing" {
				if err := os.MkdirAll(jobDir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeUnusableAttemptMetadata(t, jobDir, kind)
			job := ReadJob(testStore(), jobDir, model.JobResult{}, false)
			if got := job.DisplayStatus(model.JobSpec{ID: "job"}); got != "unknown" {
				t.Fatalf("DisplayStatus() = %q, want unknown", got)
			}
			if job.Finished() {
				t.Fatal("unusable metadata produced a terminal outcome")
			}
		})
	}
}

func TestReadJobDisplayStatusNotStarted(t *testing.T) {
	root := t.TempDir()
	runDir := filepath.Join(root, "run")
	if err := os.MkdirAll(filepath.Join(runDir, "started", "attempts"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		jobDir string
		want   string
	}{
		{name: "no job directory in an existing run", jobDir: filepath.Join(runDir, "waiting"), want: StatusNotStarted},
		{name: "missing run directory", jobDir: filepath.Join(root, "missing-run", "job"), want: "unknown"},
		{name: "missing assigned attempt", jobDir: filepath.Join(runDir, "started", "attempts", "att-1"), want: "unknown"},
		{name: "job directory without attempts", jobDir: filepath.Join(runDir, "started"), want: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			job := ReadJob(testStore(), test.jobDir, model.JobResult{}, false)
			if got := job.DisplayStatus(model.JobSpec{ID: "job"}); got != test.want {
				t.Fatalf("DisplayStatus() = %q, want %q", got, test.want)
			}
			if job.Finished() {
				t.Fatal("a job without attempts produced a terminal outcome")
			}
		})
	}
	carried := ReadJob(testStore(), filepath.Join(runDir, "carried"), model.JobResult{ID: "carried", ExitCode: 0}, true)
	if got := carried.DisplayStatus(model.JobSpec{ID: "carried"}); got != model.StatusSuccess {
		t.Fatalf("carried DisplayStatus() = %q, want success", got)
	}
}

func writeUnusableAttemptMetadata(t *testing.T, jobDir, kind string) {
	t.Helper()
	for _, name := range []string{"status", "status.json", "scheduler_status.json"} {
		path := filepath.Join(jobDir, name)
		switch kind {
		case "malformed":
			writeFile(t, path, "not valid metadata")
		case "inaccessible":
			// A symlink loop reliably fails reads even when tests run as root.
			if err := os.Symlink(name, path); err != nil {
				t.Fatal(err)
			}
		case "unrecognized":
			if name == "status.json" {
				writeFile(t, path, `{"phase":"unexpected"}`)
			} else if name == "scheduler_status.json" {
				writeFile(t, path, `{"state":"unexpected"}`)
			}
		}
	}
}

func TestReadAttemptHasAttemptRequiresDirectory(t *testing.T) {
	root := t.TempDir()
	for _, kind := range []string{"missing", "file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			jobDir := filepath.Join(root, kind, "attempts", "att-1")
			switch kind {
			case "file":
				writeFile(t, jobDir, "not a directory")
			case "directory":
				if err := os.MkdirAll(jobDir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			attempt := ReadAttempt(testStore(), jobDir)
			if attempt.HasAttempt != (kind == "directory") {
				t.Fatalf("HasAttempt = %v, want %v", attempt.HasAttempt, kind == "directory")
			}
		})
	}
}

func TestDisplayStatusCancellationRespectsTerminalResult(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		wrapper   string
		scheduler string
		summary   *model.JobResult
		want      string
	}{
		{name: "plain failure with cancelled wrapper", status: "143", wrapper: `{"phase":"cancelled","exit_code":143}`, want: model.StatusCancelled},
		{name: "normalized canceled wrapper", status: "7", wrapper: `{"phase":" CANCELED ","exit_code":7}`, want: model.StatusCancelled},
		{name: "plain success with cancelled wrapper", status: "0", wrapper: `{"phase":"cancelled","exit_code":143}`, want: model.StatusSuccess},
		{name: "successful cancelled wrapper", wrapper: `{"phase":"cancelled","exit_code":0}`, want: model.StatusSuccess},
		{name: "successful accepted result", wrapper: `{"phase":"running"}`, summary: &model.JobResult{ExitCode: 0, Accepted: true, Error: "cancelled"}, want: model.StatusSuccess},
		{name: "plain success with accepted summary", status: "0", wrapper: `{"phase":"cancelled","exit_code":143}`, summary: &model.JobResult{ExitCode: 0, Accepted: true}, want: model.StatusSuccess},
		{name: "nonterminal wrapper cannot cancel summary failure", wrapper: `{"phase":"running"}`, summary: &model.JobResult{ExitCode: 7}, want: model.StatusFailed},
		{name: "summary success beats recorded running", wrapper: `{"phase":"running"}`, summary: &model.JobResult{ExitCode: 0}, want: model.StatusSuccess},
		{name: "summary cancelled result", summary: &model.JobResult{ExitCode: 1, Error: "cancelled by user"}, want: model.StatusCancelled},
		{name: "scheduler cancelled", scheduler: "CANCELED", want: model.StatusCancelled},
		{name: "scheduler cancellation cannot override plain success", status: "0", scheduler: "cancelled", want: model.StatusSuccess},
		{name: "scheduler cancellation cannot override plain failure", status: "7", scheduler: "cancelled", want: model.StatusFailed},
		{name: "malformed wrapper preserves plain failure", status: "7", wrapper: "invalid", want: model.StatusFailed},
		{name: "unknown wrapper preserves summary success", wrapper: `{"phase":"unexpected"}`, summary: &model.JobResult{ExitCode: 0}, want: model.StatusSuccess},
		{name: "scheduler failure beats recorded running", wrapper: `{"phase":"running"}`, scheduler: "failed", want: model.StatusFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobDir := t.TempDir()
			writeDisplayStatusRecords(t, jobDir, test.status, test.wrapper, test.scheduler)
			summary := model.JobResult{}
			if test.summary != nil {
				summary = *test.summary
			}
			job := ReadJob(testStore(), jobDir, summary, test.summary != nil)
			spec := model.JobSpec{ID: "job"}
			before, beforeOK := job.Result(spec)
			if got := job.DisplayStatus(spec); got != test.want {
				t.Errorf("DisplayStatus() = %q, want %q (source %v)", got, test.want, job.Source)
			}
			if after, ok := job.Result(spec); ok != beforeOK || !reflect.DeepEqual(after, before) {
				t.Fatalf("DisplayStatus changed Result(): before %#v, %v; after %#v, %v", before, beforeOK, after, ok)
			}
		})
	}
}

func writeDisplayStatusRecords(t *testing.T, jobDir, status, wrapper, scheduler string) {
	t.Helper()
	if status != "" {
		writeFile(t, filepath.Join(jobDir, "status"), status)
	}
	if wrapper != "" {
		writeFile(t, filepath.Join(jobDir, "status.json"), wrapper)
	}
	if scheduler != "" {
		writeSchedulerState(t, jobDir, scheduler)
	}
}

func TestResolveAttemptIgnoresSummaryForOlderAttempt(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status.json"), `{"phase":"running","hosts":["old-host"]}`)
	summary := model.JobResult{ID: "job-1", AttemptID: "att-2", ExitCode: 0, Hosts: []string{"summary-host"}}
	attempt := ReadAttempt(testStore(), jobDir)

	older := ResolveAttempt(attempt, false, summary, true)
	if older.Finished() || older.HasSummary {
		t.Fatalf("ResolveAttempt(older) = %#v, want unfinished attempt without summary", older)
	}
	if _, ok := older.Result(model.JobSpec{ID: "job-1"}); ok {
		t.Fatal("older unfinished attempt returned the summary result")
	}

	latest := ResolveAttempt(attempt, true, summary, true)
	if latest.Source != SourceSummary || !latest.HasSummary {
		t.Fatalf("ResolveAttempt(latest) = %#v, want summary fallback", latest)
	}
}

// TestRecordedResultsPrefersTheSummaryToCarriedResults reads a run's
// recorded results before and after its summary exists, and for a run with
// neither file.
func TestRecordedResultsPrefersTheSummaryToCarriedResults(t *testing.T) {
	runDir := t.TempDir()
	if got := RecordedResults(runDir, nil); len(got) != 0 {
		t.Fatalf("without files: %v", got)
	}
	carried := model.RunSummary{RunID: "run-2", Results: []model.JobResult{{ID: "kept", ExitCode: 0, AttemptID: "att-1"}}}
	if err := state.WriteJSON(filepath.Join(runDir, state.CarriedResultsFileName), carried); err != nil {
		t.Fatal(err)
	}
	if got := RecordedResults(runDir, nil); len(got) != 1 || got["kept"].AttemptID != "att-1" {
		t.Fatalf("before the summary: %v", got)
	}
	summary := model.RunSummary{Results: []model.JobResult{{ID: "kept", ExitCode: 0, AttemptID: "att-1"}, {ID: "ran", ExitCode: 3}}}
	if got := RecordedResults(runDir, &summary); len(got) != 2 || got["ran"].ExitCode != 3 {
		t.Fatalf("with the summary: %v", got)
	}
}

func TestDisplayLabelAddsAcceptedAndCarried(t *testing.T) {
	for _, test := range []struct {
		status            string
		accepted, carried bool
		want              string
	}{
		{"failed", false, false, "failed"},
		{"failed", false, true, "failed (carried)"},
		{"failed", true, false, "success (accepted)"},
		{"success", true, true, "success (accepted) (carried)"},
		{"not started", false, true, "not started (carried)"},
	} {
		if got := DisplayLabel(test.status, test.accepted, test.carried); got != test.want {
			t.Errorf("DisplayLabel(%q, %t, %t) = %q, want %q", test.status, test.accepted, test.carried, got, test.want)
		}
	}
}
