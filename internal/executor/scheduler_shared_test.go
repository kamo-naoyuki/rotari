package executor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSplitShellWords(t *testing.T) {
	got, err := splitShellWords(`-p "short queue" --constraint='fast\ node' --exclusive`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-p", "short queue", "--constraint=fast\\ node", "--exclusive"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("word %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitShellWordsHandlesQuotingStyles(t *testing.T) {
	words, err := splitShellWords(`--partition "gpu queue" --constraint='a b' escaped\ value`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--partition", "gpu queue", "--constraint=a b", "escaped value"}
	if len(words) != len(want) {
		t.Fatalf("got %#v, want %#v", words, want)
	}
	for i := range want {
		if words[i] != want[i] {
			t.Errorf("word %d: got %q, want %q", i, words[i], want[i])
		}
	}
}

func TestSplitShellWordsRejectsUnterminatedInput(t *testing.T) {
	for _, input := range []string{`"unterminated`, `trailing\`, `unterminated'`} {
		if _, err := splitShellWords(input); err == nil {
			t.Errorf("splitShellWords(%q) returned nil error", input)
		}
	}
}

func TestExpandShellOptions(t *testing.T) {
	expanded, err := ExpandShellOptions([]string{"-p gpu", "--cpus-per-task=2"})
	want := []string{"-p", "gpu", "--cpus-per-task=2"}
	if err != nil || len(expanded) != len(want) {
		t.Fatalf("ExpandShellOptions = %#v, %v", expanded, err)
	}
	for i := range want {
		if expanded[i] != want[i] {
			t.Fatalf("ExpandShellOptions = %#v, want %#v", expanded, want)
		}
	}
}

func TestClassifySchedulerSubmissionFailure(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		err    error
		output string
		want   schedulerSubmissionFailureKind
	}{
		{name: "controller unavailable", err: errors.New("exit status 1"), output: "sbatch: error: controller unavailable", want: schedulerSubmissionTransient},
		{name: "transport refusal", err: errors.New("exit status 1"), output: "qsub: connection refused", want: schedulerSubmissionTransient},
		{name: "invalid partition", err: errors.New("exit status 1"), output: "sbatch: error: invalid partition", want: schedulerSubmissionPermanent},
		{name: "authorization", err: errors.New("exit status 1"), output: "bsub: permission denied", want: schedulerSubmissionPermanent},
		{name: "missing binary", err: exec.ErrNotFound, want: schedulerSubmissionPermanent},
		{name: "command timeout", err: errors.New("sbatch timed out after 30s"), want: schedulerSubmissionAmbiguous},
		{name: "missing job id", err: errors.New("qsub returned an empty job id"), want: schedulerSubmissionAmbiguous},
		{name: "unknown failure", err: errors.New("exit status 1"), output: "scheduler rejected request", want: schedulerSubmissionPermanent},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := classifySchedulerSubmissionFailure(testCase.err, []byte(testCase.output)); got != testCase.want {
				t.Fatalf("classifySchedulerSubmissionFailure(%v, %q) = %d, want %d", testCase.err, testCase.output, got, testCase.want)
			}
		})
	}
}

type schedulerSubmissionRetryTestCase struct {
	name      string
	firstErr  error
	firstText string
	wantCalls int
	wantSleep []time.Duration
}

func TestSchedulerSubmissionRetryRetriesOnlyTransientFailures(t *testing.T) {
	for _, testCase := range []schedulerSubmissionRetryTestCase{
		{name: "transient", firstErr: errors.New("exit status 1"), firstText: "controller unavailable", wantCalls: 3, wantSleep: []time.Duration{time.Second, 2 * time.Second}},
		{name: "permanent", firstErr: errors.New("exit status 1"), firstText: "invalid partition", wantCalls: 1},
		{name: "ambiguous", firstErr: errors.New("submit timed out after 30s"), wantCalls: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assertSchedulerSubmissionRetry(t, testCase)
		})
	}
}

func assertSchedulerSubmissionRetry(t *testing.T, testCase schedulerSubmissionRetryTestCase) {
	t.Helper()
	calls := 0
	var sleeps []time.Duration
	var logs []string
	policy := schedulerSubmissionRetryPolicy{
		RetryLimit: 2, InitialDelay: time.Second, MaxDelay: 30 * time.Second,
		Timing: schedulerTiming{
			Sleep:  func(delay time.Duration) { sleeps = append(sleeps, delay) },
			Jitter: func(delay time.Duration) time.Duration { return delay },
		},
	}
	output, err := policy.submit(func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }, "slurm", submissionRetryAttempt(&calls, testCase))
	assertSchedulerSubmissionRetryResult(t, testCase, output, err)
	if calls != testCase.wantCalls || !sameDurations(sleeps, testCase.wantSleep) {
		t.Fatalf("calls/sleeps = %d/%v, want %d/%v", calls, sleeps, testCase.wantCalls, testCase.wantSleep)
	}
	if len(logs) != len(testCase.wantSleep) {
		t.Fatalf("retry logs = %q, want %d entries", logs, len(testCase.wantSleep))
	}
}

func submissionRetryAttempt(calls *int, testCase schedulerSubmissionRetryTestCase) func() ([]byte, error) {
	return func() ([]byte, error) {
		*calls++
		if *calls <= 2 {
			return []byte(testCase.firstText), testCase.firstErr
		}
		return []byte("12345\n"), nil
	}
}

func assertSchedulerSubmissionRetryResult(t *testing.T, testCase schedulerSubmissionRetryTestCase, output []byte, err error) {
	t.Helper()
	if testCase.wantCalls == 1 {
		if err == nil {
			t.Fatal("submit succeeded, want original error")
		}
		return
	}
	if err != nil || string(output) != "12345\n" {
		t.Fatalf("submit = %q, %v; want success", output, err)
	}
}

func TestSchedulerSubmissionGateSpacesPerScheduler(t *testing.T) {
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	var sleeps []time.Duration
	gate := newSchedulerSubmissionGate(100 * time.Millisecond)
	gate.timing = schedulerTiming{
		Now: func() time.Time { return now },
		Sleep: func(delay time.Duration) {
			sleeps = append(sleeps, delay)
			now = now.Add(delay)
		},
	}

	gate.wait("slurm")
	gate.wait("slurm")
	gate.wait("pbs")
	gate.wait("slurm")

	want := []time.Duration{100 * time.Millisecond, 100 * time.Millisecond}
	if !sameDurations(sleeps, want) {
		t.Fatalf("submission spacing sleeps = %v, want %v", sleeps, want)
	}
}

func TestWaitForSchedulerResultUsesAccountingWhenQueueQueryIsUnavailable(t *testing.T) {
	store := testStore()
	jobDir := t.TempDir()
	queueQueries := 0
	accountingQueries := 0

	result := waitForSchedulerResult(store, jobDir, "job-1", []string{"echo", "hi"}, schedulerPollingPolicy{
		AccountingWait: time.Second, PollInterval: time.Millisecond,
		UnavailableError: "scheduler status is unavailable",
		JobState: func() schedulerQuery {
			queueQueries++
			return schedulerQuery{}
		},
		Accounting: func() schedulerAccounting {
			accountingQueries++
			return schedulerAccounting{Status: WrapperStatus{Phase: "finished", ExitCode: 7, Error: "accounting failed"}, Resolved: true}
		},
	})

	if result.ExitCode != 7 || result.Error != "accounting failed" {
		t.Fatalf("result = %+v, want accounting result", result)
	}
	if queueQueries != 1 || accountingQueries != 1 {
		t.Fatalf("queue/accounting queries = %d/%d, want 1/1", queueQueries, accountingQueries)
	}
	status, ok := LoadWrapperStatus(store, filepath.Join(jobDir, "status.json"))
	if !ok || status.Phase != "finished" || status.ExitCode != 7 {
		t.Fatalf("persisted status = %#v, ok=%v", status, ok)
	}
}

func TestWaitForSchedulerResultFailsAfterAccountingDeadline(t *testing.T) {
	queueQueries := 0
	accountingQueries := 0
	result := waitForSchedulerResult(testStore(), t.TempDir(), "job-1", []string{"echo", "hi"}, schedulerPollingPolicy{
		AccountingWait: -time.Second, PollInterval: time.Millisecond,
		UnavailableError: "scheduler status is unavailable",
		JobState: func() schedulerQuery {
			queueQueries++
			return schedulerQuery{}
		},
		Accounting: func() schedulerAccounting {
			accountingQueries++
			return schedulerAccounting{}
		},
	})

	if result.ExitCode != 1 || result.Error != "scheduler status is unavailable" {
		t.Fatalf("result = %+v, want unavailable status error", result)
	}
	if queueQueries != 1 || accountingQueries != 1 {
		t.Fatalf("queue/accounting queries = %d/%d, want 1/1", queueQueries, accountingQueries)
	}
}

func TestWaitForSchedulerResultUsesInjectedTiming(t *testing.T) {
	store := testStore()
	jobDir := t.TempDir()
	statusPath := filepath.Join(jobDir, "status.json")
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	var sleeps []time.Duration
	nowCalls := 0

	result := waitForSchedulerResult(store, jobDir, "job-1", []string{"echo", "hi"}, schedulerPollingPolicy{
		AccountingWait: time.Second, PollInterval: 250 * time.Millisecond,
		UnavailableError: "scheduler status is unavailable",
		JobState:         func() schedulerQuery { return schedulerQuery{State: "running"} },
		Accounting:       func() schedulerAccounting { return schedulerAccounting{} },
		Timing: schedulerTiming{
			Now: func() time.Time {
				nowCalls++
				return now
			},
			Sleep: func(delay time.Duration) {
				sleeps = append(sleeps, delay)
				_ = state.WriteJSON(statusPath, WrapperStatus{Phase: "finished", ExitCode: 0})
			},
			Jitter: func(delay time.Duration) time.Duration { return delay },
		},
	})

	if result.ExitCode != 0 || result.Error != "" {
		t.Fatalf("result = %+v, want successful wrapper result", result)
	}
	if nowCalls != 1 || len(sleeps) != 1 || sleeps[0] != 250*time.Millisecond {
		t.Fatalf("now calls/sleeps = %d/%v, want 1/[250ms]", nowCalls, sleeps)
	}
}

func TestSchedulerTimingDefaultsUseIdentityJitter(t *testing.T) {
	timing := (schedulerTiming{}).withDefaults()
	if timing.Now == nil || timing.Sleep == nil || timing.Jitter == nil {
		t.Fatalf("timing defaults = %#v, want all functions", timing)
	}
	if got := timing.Jitter(3 * time.Second); got != 3*time.Second {
		t.Fatalf("default jitter = %s, want 3s", got)
	}
}

func TestWaitForSchedulerResultBacksOffAfterRepeatedQueryFailures(t *testing.T) {
	store := testStore()
	jobDir := t.TempDir()
	statusPath := filepath.Join(jobDir, "status.json")
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	var sleeps, jitterInputs []time.Duration

	result := waitForSchedulerResult(store, jobDir, "job-1", []string{"echo", "hi"}, schedulerPollingPolicy{
		AccountingWait: time.Minute, PollInterval: time.Second,
		UnavailableError: "scheduler status is unavailable",
		JobState:         func() schedulerQuery { return schedulerQuery{Failed: true} },
		Accounting:       func() schedulerAccounting { return schedulerAccounting{Failed: true} },
		Timing: schedulerTiming{
			Now: func() time.Time { return now },
			Sleep: func(delay time.Duration) {
				sleeps = append(sleeps, delay)
				now = now.Add(delay)
				if len(sleeps) == 3 {
					_ = state.WriteJSON(statusPath, WrapperStatus{Phase: "finished", ExitCode: 0})
				}
			},
			Jitter: func(delay time.Duration) time.Duration {
				jitterInputs = append(jitterInputs, delay)
				return delay
			},
		},
	})

	if result.ExitCode != 0 {
		t.Fatalf("result = %+v, want successful wrapper result", result)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if !sameDurations(sleeps, want) || !sameDurations(jitterInputs, want) {
		t.Fatalf("sleeps/jitter inputs = %v/%v, want %v", sleeps, jitterInputs, want)
	}
}

func TestWaitForSchedulerResultResetsBackoffAfterSuccessfulQuery(t *testing.T) {
	store := testStore()
	jobDir := t.TempDir()
	statusPath := filepath.Join(jobDir, "status.json")
	now := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	queryCalls := 0
	var sleeps []time.Duration

	result := waitForSchedulerResult(store, jobDir, "job-1", []string{"echo", "hi"}, schedulerPollingPolicy{
		AccountingWait: time.Minute, PollInterval: time.Second,
		UnavailableError: "scheduler status is unavailable",
		JobState: func() schedulerQuery {
			queryCalls++
			if queryCalls <= 2 {
				return schedulerQuery{Failed: true}
			}
			return schedulerQuery{State: "running"}
		},
		Accounting: func() schedulerAccounting { return schedulerAccounting{Failed: true} },
		Timing: schedulerTiming{
			Now: func() time.Time { return now },
			Sleep: func(delay time.Duration) {
				sleeps = append(sleeps, delay)
				now = now.Add(delay)
				if len(sleeps) == 3 {
					_ = state.WriteJSON(statusPath, WrapperStatus{Phase: "finished", ExitCode: 0})
				}
			},
			Jitter: func(delay time.Duration) time.Duration { return delay },
		},
	})

	if result.ExitCode != 0 {
		t.Fatalf("result = %+v, want successful wrapper result", result)
	}
	want := []time.Duration{time.Second, 2 * time.Second, time.Second}
	if !sameDurations(sleeps, want) {
		t.Fatalf("sleeps = %v, want %v", sleeps, want)
	}
}

func TestSchedulerPollingDelayIsBounded(t *testing.T) {
	policy := schedulerPollingPolicy{PollInterval: time.Second}
	timing := schedulerTiming{Jitter: func(delay time.Duration) time.Duration { return delay }}
	if got := policy.pollingDelay(10, timing); got != schedulerPollingBackoffMax {
		t.Fatalf("polling delay = %s, want %s", got, schedulerPollingBackoffMax)
	}
	if got := policy.pollingDelay(1, schedulerTiming{Jitter: func(time.Duration) time.Duration { return time.Minute }}); got != schedulerPollingBackoffMax {
		t.Fatalf("jittered polling delay = %s, want %s", got, schedulerPollingBackoffMax)
	}
}

func sameDurations(left, right []time.Duration) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestSlurmArrayWrapperWritesFinishedTaskStatus(t *testing.T) {
	store := testStore()
	for _, testCase := range []struct {
		name         string
		taskVariable string
	}{
		{name: "slurm", taskVariable: "SLURM_ARRAY_TASK_ID"},
		{name: "pbs", taskVariable: "PBS_ARRAY_INDEX"},
		{name: "lsf", taskVariable: "LSB_JOBINDEX"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runDir := t.TempDir()
			task := 1
			jobDir := filepath.Join(runDir, "array-1")
			job := model.JobSpec{
				ID: "array-1", ArrayGroup: "array", ArrayTaskID: &task, ArrayFirst: 1, ArrayLast: 1,
				Command: []string{"sh", "-c", "printf task-output; exit 0"},
				Environment: []string{
					"ROTARI_ARRAY_TASK_ID=1",
					model.EnvJobDir + "=" + jobDir,
				},
			}
			wrapper := filepath.Join(runDir, "wrapper.sh")
			if err := os.WriteFile(wrapper, []byte(schedulerArrayWrapperScript([]model.JobSpec{job}, testCase.taskVariable)), 0o755); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("sh", wrapper)
			command.Env = append(os.Environ(), testCase.taskVariable+"=1")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("wrapper failed: %v, output=%s", err, output)
			}
			status, ok := LoadWrapperStatus(store, filepath.Join(jobDir, "status.json"))
			if !ok || status.Phase != "finished" || status.ExitCode != 0 {
				t.Fatalf("status = %#v, ok=%v", status, ok)
			}
			output, err := os.ReadFile(filepath.Join(jobDir, "output"))
			if err != nil || string(output) != "task-output" {
				t.Fatalf("output = %q, err=%v; want task output in job directory", output, err)
			}
		})
	}
}
