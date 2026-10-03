package runlineage

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func intPointer(value int) *int {
	return &value
}

// failedTask is a task of the array "train" with a finished result.
func failedTask(task int, status string, result model.JobResult) Job {
	id := fmt.Sprintf("train-%02d", task)
	result.ID, result.AttemptID = id, "att-"+id
	return Job{Spec: model.JobSpec{ID: id, Name: fmt.Sprintf("train[%d]", task), ArrayTaskID: intPointer(task)}, Status: status, Result: result}
}

func diagnosed(exitCode int, name, evidence, suggestion string) model.JobResult {
	return model.JobResult{ExitCode: exitCode, Error: "exit status", DiagnosisStatus: model.DiagnosisMatched,
		Diagnoses: []model.RuleDiagnosis{{Name: name, Evidence: evidence, Suggestion: suggestion}}}
}

func TestFailureGroupsClassifiesEachJobByCause(t *testing.T) {
	oom := func(evidence string) model.JobResult {
		return diagnosed(1, "CUDA/GPU memory exhausted", evidence, "Reduce batch size.")
	}
	carriedOOM := failedTask(7, StatusFailed, oom("OOM in task 7"))
	carriedOOM.Carried = true
	// A timed-out job whose log also matched a rule is grouped as a timeout.
	timedOut := failedTask(12, StatusFailed, diagnosed(model.TimeoutExitCode, "Python exception", "KeyboardInterrupt", "Inspect."))
	timedOut.Result.Error = "timed out after 5s"
	signalled := failedTask(13, StatusFailed, model.JobResult{ExitCode: 137, Error: "exit status 137", DiagnosisStatus: model.DiagnosisNoMatch})
	run := Run{Jobs: []Job{
		failedTask(1, StatusSuccess, model.JobResult{}),
		failedTask(3, StatusFailed, oom("OOM in task 3")),
		failedTask(5, StatusFailed, diagnosed(2, "Python type or value error", "ValueError: shard 5", "Fix the value.")),
		carriedOOM,
		failedTask(9, StatusFailed, diagnosed(2, "Python type or value error", "ValueError: shard 9", "Fix the value.")),
		failedTask(10, StatusUnfinished, model.JobResult{}),
		failedTask(11, StatusFailed, diagnosed(3, "CUDA/GPU memory exhausted", "OOM in task 11", "Reduce batch size.")),
		timedOut,
		signalled,
		failedTask(14, StatusFailed, model.JobResult{ExitCode: 1, Error: "cancelled by user"}),
		failedTask(15, StatusFailed, model.JobResult{ExitCode: 3, Error: "exit status 3", DiagnosisStatus: model.DiagnosisNoMatch}),
		{Spec: model.JobSpec{ID: "eval-test", Name: "eval-splittest"}, Status: StatusBlocked,
			Result: model.JobResult{ID: "eval-test", ExitCode: 1, Error: "blocked by failed dependency"}},
	}}

	got := FailureGroups(run)
	want := []FailureGroup{
		{Kind: FailureKindDiagnosis, Cause: "CUDA/GPU memory exhausted", Suggestion: "Reduce batch size.", Count: 3, Carried: 1, ExitCodes: []int{1, 3},
			Example: FailureExample{JobID: "train-03", AttemptID: "att-train-03", Evidence: "OOM in task 3"},
			Jobs: []FailureMember{
				{ID: "train-03", Name: "train[3]", ArrayTaskID: intPointer(3)},
				{ID: "train-07", Name: "train[7]", ArrayTaskID: intPointer(7), Carried: true},
				{ID: "train-11", Name: "train[11]", ArrayTaskID: intPointer(11)},
			}},
		{Kind: FailureKindDiagnosis, Cause: "Python type or value error", Suggestion: "Fix the value.", Count: 2, ExitCodes: []int{2},
			Example: FailureExample{JobID: "train-05", AttemptID: "att-train-05", Evidence: "ValueError: shard 5"},
			Jobs: []FailureMember{
				{ID: "train-05", Name: "train[5]", ArrayTaskID: intPointer(5)},
				{ID: "train-09", Name: "train[9]", ArrayTaskID: intPointer(9)},
			}},
		{Kind: model.FailureKindTimeout, Cause: model.FailureKindTimeout, Count: 1, ExitCodes: []int{model.TimeoutExitCode},
			Example: FailureExample{JobID: "train-12", AttemptID: "att-train-12", Evidence: "timed out after 5s"},
			Jobs:    []FailureMember{{ID: "train-12", Name: "train[12]", ArrayTaskID: intPointer(12)}}},
		{Kind: model.FailureKindSignal, Cause: model.FailureKindSignal, Count: 1, ExitCodes: []int{137},
			Example: FailureExample{JobID: "train-13", AttemptID: "att-train-13", Evidence: "exit status 137"},
			Jobs:    []FailureMember{{ID: "train-13", Name: "train[13]", ArrayTaskID: intPointer(13)}}},
		{Kind: model.FailureKindCancelled, Cause: model.FailureKindCancelled, Count: 1, ExitCodes: []int{1},
			Example: FailureExample{JobID: "train-14", AttemptID: "att-train-14", Evidence: "cancelled by user"},
			Jobs:    []FailureMember{{ID: "train-14", Name: "train[14]", ArrayTaskID: intPointer(14)}}},
		{Kind: model.FailureKindError, Cause: model.FailureKindError, Count: 1, ExitCodes: []int{3},
			Example: FailureExample{JobID: "train-15", AttemptID: "att-train-15", Evidence: "exit status 3"},
			Jobs:    []FailureMember{{ID: "train-15", Name: "train[15]", ArrayTaskID: intPointer(15)}}},
		{Kind: model.FailureKindBlocked, Cause: model.FailureKindBlocked, Count: 1, ExitCodes: []int{1},
			Example: FailureExample{JobID: "eval-test", Evidence: "blocked by failed dependency"},
			Jobs:    []FailureMember{{ID: "eval-test", Name: "eval-splittest"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FailureGroups() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestFailureGroupsIsEmptyWithoutFailures(t *testing.T) {
	got := FailureGroups(Run{Jobs: []Job{failedTask(1, StatusSuccess, model.JobResult{}), failedTask(2, StatusUnfinished, model.JobResult{})}})
	if len(got) != 0 {
		t.Fatalf("FailureGroups() = %+v, want none", got)
	}
}

func TestCompareReportsFailureCauses(t *testing.T) {
	oom := diagnosed(1, "CUDA/GPU memory exhausted", "OOM", "Reduce batch size.")
	withResult := func(name, status string, result model.JobResult) Job {
		return Job{Spec: model.JobSpec{ID: name + "-id", Name: name}, Status: status, Result: result}
	}
	timedOut := model.JobResult{ExitCode: model.TimeoutExitCode, Error: "timed out after 5s"}
	valueError := diagnosed(2, "Python type or value error", "ValueError", "Fix the value.")
	from := Run{ID: "run-1", Jobs: []Job{
		withResult("changed", StatusFailed, oom),
		withResult("same", StatusFailed, oom),
		withResult("fixed", StatusFailed, oom),
		withResult("newly", StatusSuccess, model.JobResult{}),
	}}
	to := Run{ID: "run-2", Jobs: []Job{
		withResult("changed", StatusFailed, timedOut),
		withResult("same", StatusFailed, diagnosed(3, "CUDA/GPU memory exhausted", "OOM again", "Reduce batch size.")),
		withResult("fixed", StatusSuccess, model.JobResult{}),
		withResult("newly", StatusFailed, valueError),
	}}
	result := Compare(from, to)
	got := map[string][3]string{}
	for _, job := range result.Jobs {
		got[job.Name] = [3]string{job.FromCause, job.ToCause, fmt.Sprint(job.CauseChanged)}
	}
	want := map[string][3]string{
		"changed": {"CUDA/GPU memory exhausted", model.FailureKindTimeout, "true"},
		"same":    {"CUDA/GPU memory exhausted", "CUDA/GPU memory exhausted", "false"},
		"fixed":   {"CUDA/GPU memory exhausted", "", "false"},
		"newly":   {"", "Python type or value error", "false"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("causes = %v\nwant %v", got, want)
	}
	if result.Summary.CauseChanged != 1 || result.Summary.StillFailing != 2 {
		t.Fatalf("summary = %+v, want 2 still failing with 1 changed cause", result.Summary)
	}
}

func TestLimitMembersKeepsTheFirstJobsAndCountsTheRest(t *testing.T) {
	members := func(n int) []FailureMember {
		jobs := make([]FailureMember, n)
		for index := range jobs {
			jobs[index] = FailureMember{ID: fmt.Sprintf("job-%d", index)}
		}
		return jobs
	}
	groups := []FailureGroup{{Count: 12, Jobs: members(12)}, {Count: 3, Jobs: members(3)}}
	LimitMembers(groups, 10)
	if len(groups[0].Jobs) != 10 || groups[0].JobsOmitted != 2 || groups[0].Jobs[9].ID != "job-9" || groups[0].Count != 12 {
		t.Errorf("large group = %d jobs, %d omitted, count %d", len(groups[0].Jobs), groups[0].JobsOmitted, groups[0].Count)
	}
	if len(groups[1].Jobs) != 3 || groups[1].JobsOmitted != 0 {
		t.Errorf("small group = %d jobs, %d omitted", len(groups[1].Jobs), groups[1].JobsOmitted)
	}
}
