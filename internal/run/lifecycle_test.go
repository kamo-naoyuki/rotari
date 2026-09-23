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
