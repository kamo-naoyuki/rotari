package model

import "testing"

func TestQueueOriginsByJobIDIncludesCommandsAndArrayTasks(t *testing.T) {
	commandOrigin := &JobOrigin{RunID: "run-1", JobID: "job-1"}
	taskOrigin := &JobOrigin{RunID: "run-1", JobID: "array-2"}
	origins := QueueOriginsByJobID(Queue{Commands: []QueuedCommand{
		{ID: "job-1", Origin: commandOrigin},
		{ID: "array", TaskOrigins: map[string]*JobOrigin{"array-2": taskOrigin}},
	}})

	if origins["job-1"] != commandOrigin || origins["array-2"] != taskOrigin {
		t.Fatalf("origins = %#v, want command and task origins", origins)
	}
}
