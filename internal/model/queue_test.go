package model

import "testing"

func TestQueueToJobsPreservesName(t *testing.T) {
	jobs := QueueToJobs([]QueuedCommand{{ID: "fixed-id", Command: []string{"echo", "hello"}, Name: "greeting"}})
	if len(jobs) != 1 || jobs[0].ID != "fixed-id" || jobs[0].Name != "greeting" {
		t.Fatalf("jobs = %#v, want named scalar job", jobs)
	}
}

func TestQueueToJobsExpandsDenseAndSparseArrays(t *testing.T) {
	tests := []struct {
		name  string
		array *ArraySpec
		ids   []string
	}{
		{name: "dense", array: &ArraySpec{First: 2, Last: 4}, ids: []string{"array-2", "array-3", "array-4"}},
		{name: "sparse", array: &ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3, 4}}, ids: []string{"array-1", "array-3", "array-4"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jobs := QueueToJobs([]QueuedCommand{{ID: "array", Command: []string{"echo", "hello"}, Array: test.array}})
			if len(jobs) != len(test.ids) {
				t.Fatalf("got %d jobs, want %d", len(jobs), len(test.ids))
			}
			for index, id := range test.ids {
				if jobs[index].ID != id || jobs[index].ArraySize != len(test.ids) {
					t.Fatalf("job %d = %#v, want %s with size %d", index, jobs[index], id, len(test.ids))
				}
			}
		})
	}
}

func TestQueueToJobsExpandsStageDependency(t *testing.T) {
	commands := []QueuedCommand{
		{ID: "prepare-a", Stage: "prepare", Command: []string{"prepare-a"}},
		{ID: "prepare-b", Stage: "prepare", Command: []string{"prepare-b"}},
		{ID: "train", Name: "train", DependsOn: []string{"prepare"}, Command: []string{"train"}},
	}
	jobs := QueueToJobs(commands)
	if got, want := jobs[2].DependsOn, []string{"prepare-a", "prepare-b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("dependencies = %v, want %v", got, want)
	}
	if err := ValidateQueueDependencies(commands); err != nil {
		t.Fatalf("ValidateQueueDependencies() error = %v", err)
	}
}

func TestValidateQueueDependenciesRejectsStageJobNameConflict(t *testing.T) {
	err := ValidateQueueDependencies([]QueuedCommand{
		{ID: "prepare", Name: "build", Command: []string{"prepare"}},
		{ID: "compile", Stage: "build", Command: []string{"compile"}},
	})
	if err == nil || err.Error() != "job name conflicts with stage name: build" {
		t.Fatalf("ValidateQueueDependencies() error = %v, want stage/job name conflict", err)
	}
}
