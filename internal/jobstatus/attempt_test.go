package jobstatus

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func testStore() state.Store {
	return state.NewStore(0o700, 0o600)
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSchedulerState(t *testing.T, jobDir, value string) {
	t.Helper()
	executor.WriteSchedulerStatus(testStore(), jobDir, value, time.Now())
}

func TestReadAttemptFallbackChain(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		wrapper   string
		scheduler string
		want      Source
		wantCode  int
	}{
		{name: "none", want: SourceNone},
		{name: "status file wins", status: "7\n", wrapper: `{"phase":"finished","exit_code":3}`, scheduler: "failed", want: SourceStatus, wantCode: 7},
		{name: "terminal wrapper", wrapper: `{"phase":"finished","exit_code":3}`, scheduler: "completed", want: SourceWrapper, wantCode: 3},
		{name: "wrapper finished_at marker", wrapper: `{"phase":"running","exit_code":4,"finished_at":"2026-09-18T00:00:00Z"}`, want: SourceWrapper, wantCode: 4},
		{name: "running wrapper falls back to scheduler", wrapper: `{"phase":"running"}`, scheduler: "failed", want: SourceScheduler, wantCode: 1},
		{name: "completed scheduler", scheduler: "COMPLETED", want: SourceScheduler, wantCode: 0},
		{name: "running scheduler", scheduler: "RUNNING", want: SourceNone},
		{name: "invalid status file", status: "oops", want: SourceNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobDir := t.TempDir()
			if test.status != "" {
				writeFile(t, filepath.Join(jobDir, "status"), test.status)
			}
			if test.wrapper != "" {
				writeFile(t, filepath.Join(jobDir, "status.json"), test.wrapper)
			}
			if test.scheduler != "" {
				writeSchedulerState(t, jobDir, test.scheduler)
			}
			attempt := ReadAttempt(testStore(), jobDir)
			if attempt.Source != test.want || attempt.ExitCode != test.wantCode {
				t.Fatalf("ReadAttempt() = source %d exit %d, want source %d exit %d", attempt.Source, attempt.ExitCode, test.want, test.wantCode)
			}
			if attempt.Finished() != (test.want != SourceNone) {
				t.Fatalf("Finished() = %v", attempt.Finished())
			}
		})
	}
}

func TestReadAttemptKeepsNonTerminalWrapper(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status.json"), `{"phase":"running","hosts":["node1"]}`)
	attempt := ReadAttempt(testStore(), jobDir)
	if attempt.Finished() || !attempt.HasWrapper || attempt.Wrapper.Phase != "running" {
		t.Fatalf("ReadAttempt() = %#v, want unfinished attempt with wrapper phase", attempt)
	}
	if _, ok := attempt.Result(model.JobSpec{ID: "job-1"}); ok {
		t.Fatal("unfinished attempt returned a result")
	}
}

func TestAttemptResultCarriesWrapperDetails(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status.json"), `{"phase":"finished","exit_code":2,"error":"boom","hosts":["node1"]}`)
	job := model.JobSpec{ID: "job-1", Command: []string{"false"}}
	result, ok := ReadAttempt(testStore(), jobDir).Result(job)
	want := model.JobResult{ID: "job-1", Command: []string{"false"}, ExitCode: 2, Error: "boom", Hosts: []string{"node1"}}
	if !ok || !reflect.DeepEqual(result, want) {
		t.Fatalf("Result() = %#v, %v, want %#v", result, ok, want)
	}

	writeFile(t, filepath.Join(jobDir, "status"), "0\n")
	result, ok = ReadAttempt(testStore(), jobDir).Result(job)
	if !ok || result.ExitCode != 0 || result.Error != "" || !reflect.DeepEqual(result.Hosts, []string{"node1"}) {
		t.Fatalf("Result() from status file = %#v, %v, want exit 0 with wrapper hosts and no wrapper error", result, ok)
	}
}

func TestReadStatusFileRejectsUnsafeDirectory(t *testing.T) {
	if _, ok := ReadStatusFile(""); ok {
		t.Fatal("ReadStatusFile() accepted an empty directory")
	}
}

func TestWrapperTerminalUsesFinishedAtMarker(t *testing.T) {
	if !WrapperTerminal(executor.WrapperStatus{Phase: "running", FinishedAt: "2026-09-18T00:00:00Z"}) {
		t.Fatal("status with finished_at was not treated as terminal")
	}
	if WrapperTerminal(executor.WrapperStatus{Phase: "running"}) {
		t.Fatal("running status without finished_at was treated as terminal")
	}
}
