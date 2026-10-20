package model

import (
	"reflect"
	"testing"
)

func TestTaskOrigin(t *testing.T) {
	group := &JobOrigin{RunID: "original-run", JobID: "original-array"}
	task := &JobOrigin{RunID: "other-run", JobID: "specific-task", AttemptID: "specific-attempt"}
	command := QueuedCommand{
		ID: "copy-array", Array: &ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3, 4}},
		Origin: group, TaskOrigins: map[string]*JobOrigin{"copy-array-3": task, "copy-array-4": nil},
	}
	for _, test := range []struct {
		name   string
		task   int
		origin *JobOrigin
		want   *JobOrigin
	}{
		{"inherited", 1, group, &JobOrigin{RunID: group.RunID, JobID: "original-array-1"}},
		{"explicit", 3, group, task},
		{"nil task origin inherits", 4, group, &JobOrigin{RunID: group.RunID, JobID: "original-array-4"}},
		{"no origin", 1, nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			command.Origin = test.origin
			if got := TaskOrigin(command, test.task); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("TaskOrigin(%d)=%+v want %+v", test.task, got, test.want)
			}
			if group.JobID != "original-array" {
				t.Fatal("changed array origin while resolving a task")
			}
		})
	}
}
