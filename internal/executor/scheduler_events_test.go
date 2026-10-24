package executor

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rotari-dev/rotari/internal/state"
)

var errTestSchedulerQuery = errors.New("exit status 1: slurm_load_jobs error: Socket timed out on send/recv operation")

func readSchedulerEvents(t *testing.T, jobDir string) []SchedulerEvent {
	t.Helper()
	file, err := os.Open(filepath.Join(jobDir, state.SchedulerEventsFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var events []SchedulerEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event SchedulerEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("event line %q: %v", scanner.Text(), err)
		}
		events = append(events, event)
	}
	return events
}

func schedulerEventSummary(events []SchedulerEvent) []string {
	summary := make([]string, 0, len(events))
	for _, event := range events {
		line := event.At[11:19] + " " + event.Query + " " + event.State
		if event.Error != "" {
			line += " error=" + event.Error
		}
		summary = append(summary, line)
	}
	return summary
}

func assertSchedulerEvents(t *testing.T, jobDir string, want []string) []SchedulerEvent {
	t.Helper()
	events := readSchedulerEvents(t, jobDir)
	got := schedulerEventSummary(events)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("scheduler events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	return events
}

// The shape of the 2026-10-14 Slurm report: squeue fails for a while,
// sacct shows the job running, then squeue recovers. Each change is logged
// once, with the query error, however many polls it lasts.
func TestSchedulerEventsRecordQueueOutageAndRecovery(t *testing.T) {
	store := testStore()
	jobDir := t.TempDir()
	statusPath := filepath.Join(jobDir, "status.json")
	now := time.Date(2026, time.October, 14, 9, 0, 0, 0, time.UTC)
	queries := 0

	waitForSchedulerResult(store, jobDir, "job-1", []string{"train"}, schedulerPollingPolicy{
		AccountingWait: time.Minute, PollInterval: time.Second,
		JobState: func() schedulerQuery {
			queries++
			if queries <= 4 {
				return schedulerQuery{Err: errTestSchedulerQuery}
			}
			return schedulerQuery{State: "running"}
		},
		Accounting: func() schedulerAccounting { return schedulerAccounting{State: "running"} },
		Timing: schedulerTiming{
			Now: func() time.Time { return now },
			Sleep: func(time.Duration) {
				now = now.Add(time.Minute)
				if queries == 6 {
					_ = state.WriteJSON(statusPath, WrapperStatus{Phase: "finished", ExitCode: 3, Error: "exit status 3"})
				}
			},
			Jitter: func(delay time.Duration) time.Duration { return delay },
		},
	})

	events := assertSchedulerEvents(t, jobDir, []string{
		"09:00:00 queue  error=" + errTestSchedulerQuery.Error(),
		"09:00:00 accounting running",
		"09:04:00 queue running",
		"09:06:00 result wrapper error=exit status 3",
	})
	if code := events[len(events)-1].ExitCode; code == nil || *code != 3 {
		t.Fatalf("result exit code = %v, want 3", code)
	}
}

func TestSchedulerEventsRecordEachResultSource(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		accounting schedulerAccounting
		want       []string
	}{
		{
			name:       "accounting",
			accounting: schedulerAccounting{Status: WrapperStatus{Phase: "completed", ExitCode: 0}, Resolved: true},
			want:       []string{"12:00:00 queue missing", "12:00:00 accounting completed", "12:00:00 result accounting"},
		},
		{
			name:       "unavailable",
			accounting: schedulerAccounting{},
			want:       []string{"12:00:00 queue missing", "12:00:00 accounting missing", "12:00:00 result unavailable error=accounting is unavailable"},
		},
		{
			name:       "accounting error then unavailable",
			accounting: schedulerAccounting{Err: errors.New("sacct timed out after 30s")},
			want:       []string{"12:00:00 queue missing", "12:00:00 accounting  error=sacct timed out after 30s", "12:00:00 result unavailable error=accounting is unavailable"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			jobDir := t.TempDir()
			waitForSchedulerResult(testStore(), jobDir, "job-1", []string{"train"}, schedulerPollingPolicy{
				AccountingWait: -time.Second, PollInterval: time.Second,
				UnavailableError: "accounting is unavailable",
				JobState:         func() schedulerQuery { return schedulerQuery{} },
				Accounting:       func() schedulerAccounting { return testCase.accounting },
				Timing: schedulerTiming{
					Now:    func() time.Time { return time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC) },
					Sleep:  func(time.Duration) {},
					Jitter: func(delay time.Duration) time.Duration { return delay },
				},
			})
			assertSchedulerEvents(t, jobDir, testCase.want)
		})
	}
}

func TestSchedulerEventsStopAtSizeLimit(t *testing.T) {
	jobDir := t.TempDir()
	path := filepath.Join(jobDir, state.SchedulerEventsFileName)
	if err := os.WriteFile(path, []byte(strings.Repeat("x", schedulerEventsSizeLimit)), 0o600); err != nil {
		t.Fatal(err)
	}
	log := newSchedulerEventLog(testStore(), jobDir, time.Now)
	log.observe("queue", "running", nil)
	log.result("wrapper", 0, "")
	if info, err := os.Stat(path); err != nil || info.Size() != schedulerEventsSizeLimit {
		t.Fatalf("event log size = %v, %v; want it left at the limit", info, err)
	}
}

func TestSchedulerEventsCutLongErrors(t *testing.T) {
	jobDir := t.TempDir()
	log := newSchedulerEventLog(testStore(), jobDir, time.Now)
	log.observe("queue", "", errors.New(strings.Repeat("e", 2*schedulerEventsErrorLimit)))
	events := readSchedulerEvents(t, jobDir)
	if len(events) != 1 || len(events[0].Error) != schedulerEventsErrorLimit+len("...") {
		t.Fatalf("events = %#v, want one error cut to the limit", events)
	}
}

func TestSlurmQueryErrorsKeepCommandOutput(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "squeue", "#!/bin/sh\necho 'slurm_load_jobs error: Socket timed out' >&2\nexit 1\n")
	writeExecutable(t, binDir, "sacct", "#!/bin/sh\necho 'sacct: error: slurmdbd: Connection refused' >&2\nexit 1\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := slurmJobState("12345"); err == nil || !strings.Contains(err.Error(), "Socket timed out") {
		t.Fatalf("slurmJobState error = %v, want squeue's message", err)
	}
	if _, _, _, err := slurmAccounting("12345"); err == nil || !strings.Contains(err.Error(), "Connection refused") {
		t.Fatalf("slurmAccounting error = %v, want sacct's message", err)
	}
}
