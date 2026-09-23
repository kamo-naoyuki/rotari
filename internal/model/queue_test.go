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
