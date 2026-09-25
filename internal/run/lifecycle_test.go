package run

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestExecuteDependencyRetriesRunsWavesAndRetries(t *testing.T) {
	jobs := []model.JobSpec{{ID: "first"}, {ID: "second", DependsOn: []string{"first"}}}
	results := make(map[string]model.JobResult)
	calls := 0
	pending := ExecuteDependencyRetries(jobs, map[string]model.JobSpec{"first": jobs[0], "second": jobs[1]}, results, 0, AttemptCallbacks{
		Execute: func(_ int, ready []model.JobSpec) []model.JobResult {
			calls++
			result := make([]model.JobResult, 0, len(ready))
			for _, job := range ready {
				result = append(result, model.JobResult{ID: job.ID, Command: job.Command})
			}
			return result
		},
	})
	if len(pending) != 0 || calls != 2 || len(results) != 2 {
		t.Fatalf("pending=%#v calls=%d results=%#v, want two waves and no pending", pending, calls, results)
	}
}

func TestExecuteDependencyRetriesReportsRetryAndFinalFailure(t *testing.T) {
	job := model.JobSpec{ID: "job-1"}
	results := make(map[string]model.JobResult)
	var progress []model.JobResult
	attempt := 0
	ExecuteDependencyRetries([]model.JobSpec{job}, map[string]model.JobSpec{}, results, 1, AttemptCallbacks{
		Execute: func(_ int, jobs []model.JobSpec) []model.JobResult {
			attempt++
			return []model.JobResult{{ID: jobs[0].ID, ExitCode: 1}}
		},
		Progress: func(result model.JobResult, _, _, _, _ int) {
			progress = append(progress, result)
		},
	})
	if attempt != 2 || len(progress) != 3 {
		t.Fatalf("attempts=%d progress=%#v, want two attempts and retry/final events", attempt, progress)
	}
	if progress[0].Error != "retry:1" || progress[len(progress)-1].Error != "final-failure" {
		t.Fatalf("progress=%#v, want retry:1 then final-failure", progress)
	}
}

// runAttempts executes jobs with results decided by exitCodes (job ID to exit
// code per attempt; missing means success) and records the order jobs ran in.
func runAttempts(t *testing.T, jobs []model.JobSpec, retry int, exitCodes map[string][]int) (map[string]model.JobResult, []string) {
	t.Helper()
	byName := make(map[string]model.JobSpec, len(jobs))
	for _, job := range jobs {
		byName[job.Name] = job
	}
	results := make(map[string]model.JobResult)
	runs := make(map[string]int)
	var order []string
	pending := ExecuteDependencyRetries(jobs, byName, results, retry, AttemptCallbacks{
		Execute: func(_ int, ready []model.JobSpec) []model.JobResult {
			waveResults := make([]model.JobResult, 0, len(ready))
			for _, job := range ready {
				exitCode := 0
				if codes := exitCodes[job.ID]; runs[job.ID] < len(codes) {
					exitCode = codes[runs[job.ID]]
				}
				runs[job.ID]++
				order = append(order, job.ID)
				waveResults = append(waveResults, model.JobResult{ID: job.ID, ExitCode: exitCode})
			}
			return waveResults
		},
	})
	FinalizePendingResults(pending, results)
	return results, order
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
