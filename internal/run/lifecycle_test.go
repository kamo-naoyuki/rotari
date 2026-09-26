package run

import (
	"sync"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// runAttempts executes jobs with results decided by exitCodes (job ID to exit
// code per attempt; missing means success) and records the order jobs ran in.
// Retries are immediate.
func runAttempts(t *testing.T, jobs []model.JobSpec, retry int, exitCodes map[string][]int) (map[string]model.JobResult, []string) {
	t.Helper()
	byName := make(map[string]model.JobSpec, len(jobs))
	for _, job := range jobs {
		byName[job.Name] = job
	}
	results := make(map[string]model.JobResult)
	runs := make(map[string]int)
	var order []string
	var mu sync.Mutex
	pending := ExecuteJobs(jobs, byName, results, EngineOptions{
		RunRetry: retry,
		Start: func(ready []model.JobSpec, done func(model.JobResult)) {
			go func() {
				for _, job := range ready {
					mu.Lock()
					exitCode := 0
					if codes := exitCodes[job.ID]; runs[job.ID] < len(codes) {
						exitCode = codes[runs[job.ID]]
					}
					runs[job.ID]++
					order = append(order, job.ID)
					mu.Unlock()
					done(model.JobResult{ID: job.ID, AttemptID: job.AttemptID, ExitCode: exitCode})
				}
			}()
		},
	})
	FinalizePendingResults(pending, results)
	return results, order
}

func TestExecuteJobsRunsDependenciesAndReportsRetries(t *testing.T) {
	jobs := []model.JobSpec{{ID: "first", Name: "first"}, {ID: "second", Name: "second", DependsOn: []string{"first"}}}
	var progress []string
	var mu sync.Mutex
	results := make(map[string]model.JobResult)
	ExecuteJobs(jobs, map[string]model.JobSpec{"first": jobs[0], "second": jobs[1]}, results, EngineOptions{
		RunRetry: 1,
		Start: func(ready []model.JobSpec, done func(model.JobResult)) {
			for _, job := range ready {
				exitCode := 0
				if job.ID == "first" {
					exitCode = 1
				}
				go done(model.JobResult{ID: job.ID, ExitCode: exitCode})
			}
		},
		Progress: func(result model.JobResult, _, _, _, _ int) {
			mu.Lock()
			progress = append(progress, result.ID+":"+result.Error)
			mu.Unlock()
		},
	})
	want := []string{"first:retry:1", "first:", "first:final-failure"}
	if len(progress) != len(want) || progress[0] != want[0] || progress[2] != want[2] {
		t.Fatalf("progress = %v, want %v", progress, want)
	}
	if results["second"].Error != "blocked by failed dependency" {
		t.Fatalf("second = %#v, want blocked after first failed for good", results["second"])
	}
}

// gatedStart runs jobs on goroutines; a job listed in hold keeps running
// until its channel is closed, and every job fails on its first attempt when
// listed in failOnce.
type gatedStart struct {
	mu       sync.Mutex
	hold     map[string]chan struct{}
	failOnce map[string]bool
	started  []string
}

func (gate *gatedStart) start(ready []model.JobSpec, done func(model.JobResult)) {
	for _, job := range ready {
		gate.mu.Lock()
		gate.started = append(gate.started, job.ID)
		release := gate.hold[job.ID]
		exitCode := 0
		if gate.failOnce[job.ID] {
			exitCode = 1
			delete(gate.failOnce, job.ID)
		}
		gate.mu.Unlock()
		go func(id string) {
			if release != nil {
				<-release
			}
			done(model.JobResult{ID: id, ExitCode: exitCode})
		}(job.ID)
	}
}

func (gate *gatedStart) startedJobs() []string {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return append([]string(nil), gate.started...)
}

func waitFor(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal(message)
		}
		time.Sleep(time.Millisecond)
	}
}

func count(values []string, value string) int {
	total := 0
	for _, candidate := range values {
		if candidate == value {
			total++
		}
	}
	return total
}

func TestExecuteJobsProgressCountsOnlyFinalExecutedResults(t *testing.T) {
	jobs := []model.JobSpec{{ID: "flaky", Name: "flaky"}, {ID: "failing", Name: "failing"}}
	results := map[string]model.JobResult{
		"carried-ok":   {ID: "carried-ok", ExitCode: 0},
		"carried-fail": {ID: "carried-fail", ExitCode: 1},
	}
	var progress [][4]int
	var mu sync.Mutex
	attempts := make(map[string]int)
	ExecuteJobs(jobs, map[string]model.JobSpec{"flaky": jobs[0], "failing": jobs[1]}, results, EngineOptions{
		RunRetry: 1,
		Start: func(ready []model.JobSpec, done func(model.JobResult)) {
			for _, job := range ready {
				mu.Lock()
				attempts[job.ID]++
				exitCode := 1
				if job.ID == "flaky" && attempts[job.ID] > 1 {
					exitCode = 0
				}
				mu.Unlock()
				go done(model.JobResult{ID: job.ID, ExitCode: exitCode})
			}
		},
		Progress: func(_ model.JobResult, completed, total, succeeded, failed int) {
			mu.Lock()
			progress = append(progress, [4]int{completed, total, succeeded, failed})
			mu.Unlock()
		},
	})
	for _, counts := range progress {
		if counts[0] > counts[1] || counts[0] != counts[2]+counts[3] {
			t.Fatalf("progress = %v, want completed within total and equal to succeeded+failed", progress)
		}
	}
	if last := progress[len(progress)-1]; last != [4]int{2, 2, 1, 1} {
		t.Fatalf("final progress = %v, want completed=2 total=2 succeeded=1 failed=1", last)
	}
}

func TestExecuteJobsRetriesAndUnblocksWithoutWaitingForOtherJobs(t *testing.T) {
	gate := &gatedStart{hold: map[string]chan struct{}{"slow": make(chan struct{})}, failOnce: map[string]bool{"flaky": true}}
	jobs := []model.JobSpec{
		{ID: "slow", Name: "slow"},
		{ID: "flaky", Name: "flaky"},
		{ID: "next", Name: "next", DependsOn: []string{"flaky"}},
	}
	byName := map[string]model.JobSpec{"slow": jobs[0], "flaky": jobs[1], "next": jobs[2]}
	finished := make(chan map[string]model.JobResult)
	go func() {
		results := make(map[string]model.JobResult)
		ExecuteJobs(jobs, byName, results, EngineOptions{RunRetry: 1, Start: gate.start})
		finished <- results
	}()
	// While "slow" is still running, "flaky" is retried and "next" starts.
	waitFor(t, func() bool {
		started := gate.startedJobs()
		return count(started, "flaky") == 2 && count(started, "next") == 1
	}, "flaky was not retried and next did not start while slow was running")
	close(gate.hold["slow"])
	results := <-finished
	if results["flaky"].ExitCode != 0 || results["next"].ExitCode != 0 || results["slow"].ExitCode != 0 {
		t.Fatalf("results = %#v", results)
	}
}

func TestExecuteJobsSpacesRetriesWithBackoff(t *testing.T) {
	retries := 3
	job := model.JobSpec{ID: "job", Name: "job", Retry: &retries, RetryDelay: "10s", RetryBackoff: 2, RetryMaxDelay: "30s"}
	var delays []time.Duration
	var attemptIDs []string
	results := make(map[string]model.JobResult)
	ExecuteJobs([]model.JobSpec{job}, map[string]model.JobSpec{"job": job}, results, EngineOptions{
		Start: func(ready []model.JobSpec, done func(model.JobResult)) {
			attemptIDs = append(attemptIDs, ready[0].AttemptID)
			go done(model.JobResult{ID: "job", ExitCode: 1})
		},
		AssignAttemptID: func(job *model.JobSpec, attempt int) { job.AttemptID = string(rune('a' + attempt)) },
		After: func(delay time.Duration, f func()) {
			delays = append(delays, delay)
			go f()
		},
	})
	want := []time.Duration{10 * time.Second, 20 * time.Second, 30 * time.Second}
	if len(delays) != len(want) || delays[0] != want[0] || delays[1] != want[1] || delays[2] != want[2] {
		t.Fatalf("retry delays = %v, want %v", delays, want)
	}
	if len(attemptIDs) != 4 || attemptIDs[0] != "a" || attemptIDs[3] != "d" {
		t.Fatalf("attempt IDs = %v, want one per attempt numbered per job", attemptIDs)
	}
}

func TestExecuteJobsStopsRetryingWhenStopped(t *testing.T) {
	jobs := []model.JobSpec{{ID: "failing", Name: "failing"}, {ID: "later", Name: "later", DependsOnFinished: []string{"failing"}}}
	stopped := false
	results := make(map[string]model.JobResult)
	pending := ExecuteJobs(jobs, map[string]model.JobSpec{"failing": jobs[0], "later": jobs[1]}, results, EngineOptions{
		RunRetry: 5,
		Start: func(ready []model.JobSpec, done func(model.JobResult)) {
			stopped = true
			go done(model.JobResult{ID: ready[0].ID, ExitCode: 1})
		},
		Stopped: func() bool { return stopped },
	})
	if len(pending) != 0 || results["failing"].ExitCode != 1 {
		t.Fatalf("pending=%v results=%#v, want no retry after the run stopped", pending, results)
	}
	if results["later"].Error != cancelledBeforeStart {
		t.Fatalf("later = %#v, want it cancelled before start", results["later"])
	}
}

func TestFinishedDependencyRunsAfterPrerequisiteFails(t *testing.T) {
	jobs := []model.JobSpec{
		{ID: "sweep", Name: "sweep"},
		{ID: "strict", Name: "strict", DependsOn: []string{"sweep"}},
		{ID: "collect", Name: "collect", DependsOnFinished: []string{"sweep"}},
	}
	results, order := runAttempts(t, jobs, 0, map[string][]int{"sweep": {1}})
	if results["collect"].ExitCode != 0 || results["collect"].Error != "" {
		t.Fatalf("collect result = %#v, want it to run after sweep failed", results["collect"])
	}
	if results["strict"].Error != "blocked by failed dependency" {
		t.Fatalf("strict result = %#v, want blocked", results["strict"])
	}
	if len(order) != 2 || order[0] != "sweep" || order[1] != "collect" {
		t.Fatalf("execution order = %v, want sweep then collect", order)
	}
}

func TestFinishedDependencyWaitsForRetries(t *testing.T) {
	jobs := []model.JobSpec{
		{ID: "sweep", Name: "sweep"},
		{ID: "collect", Name: "collect", DependsOnFinished: []string{"sweep"}},
	}
	results, order := runAttempts(t, jobs, 1, map[string][]int{"sweep": {1, 1}})
	want := []string{"sweep", "sweep", "collect"}
	if len(order) != len(want) || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] {
		t.Fatalf("execution order = %v, want %v (collect waits for the retry)", order, want)
	}
	if results["collect"].ExitCode != 0 {
		t.Fatalf("collect result = %#v, want success", results["collect"])
	}
	results, order = runAttempts(t, jobs, 1, map[string][]int{"sweep": {1, 0}})
	if len(order) != 3 || order[2] != "collect" || results["sweep"].ExitCode != 0 {
		t.Fatalf("order=%v sweep=%#v, want collect after the successful retry", order, results["sweep"])
	}
}

func TestFinishedDependencyRunsAfterBlockedPrerequisite(t *testing.T) {
	jobs := []model.JobSpec{
		{ID: "prepare", Name: "prepare"},
		{ID: "train", Name: "train", DependsOn: []string{"prepare"}},
		{ID: "cleanup", Name: "cleanup", DependsOnFinished: []string{"train"}},
	}
	results, order := runAttempts(t, jobs, 0, map[string][]int{"prepare": {1}})
	if results["train"].Error != "blocked by failed dependency" {
		t.Fatalf("train result = %#v, want blocked", results["train"])
	}
	if results["cleanup"].ExitCode != 0 || len(order) != 2 || order[1] != "cleanup" {
		t.Fatalf("cleanup result=%#v order=%v, want cleanup to run after train was blocked", results["cleanup"], order)
	}
}

func TestPerJobRetryOverridesRunRetry(t *testing.T) {
	two, zero := 2, 0
	jobs := []model.JobSpec{
		{ID: "flaky", Name: "flaky", Retry: &two},
		{ID: "strict", Name: "strict", Retry: &zero},
		{ID: "default", Name: "default"},
	}
	results, order := runAttempts(t, jobs, 1, map[string][]int{
		"flaky": {1, 1, 0}, "strict": {1, 0}, "default": {1, 1, 0},
	})
	runs := map[string]int{}
	for _, id := range order {
		runs[id]++
	}
	if runs["flaky"] != 3 || results["flaky"].ExitCode != 0 {
		t.Fatalf("flaky ran %d times with %#v, want 3 runs ending in success", runs["flaky"], results["flaky"])
	}
	if runs["strict"] != 1 || results["strict"].ExitCode != 1 {
		t.Fatalf("strict ran %d times, want no retry", runs["strict"])
	}
	if runs["default"] != 2 || results["default"].ExitCode != 1 {
		t.Fatalf("default ran %d times, want the run's single retry", runs["default"])
	}
}
