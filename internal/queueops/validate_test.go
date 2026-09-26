package queueops

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestValidateJobsRejectsInvalidArrayDefinitions(t *testing.T) {
	tests := []struct {
		name  string
		array model.ArraySpec
		want  string
	}{
		{name: "negative", array: model.ArraySpec{First: -1, Last: 1}, want: "task indexes must not be negative"},
		{name: "reversed", array: model.ArraySpec{First: 4, Last: 2}, want: "first index must not be greater than last index"},
		{name: "bounds mismatch", array: model.ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3}}, want: "first and last indexes must match"},
		{name: "out of range", array: model.ArraySpec{First: 1, Last: 4, Tasks: []int{1, 5, 4}}, want: "task index 5 is outside 1-4"},
		{name: "duplicate", array: model.ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3, 3, 4}}, want: "task indexes must be strictly increasing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queue := model.Queue{Commands: []model.QueuedCommand{{ID: "array", Command: []string{"true"}, Array: &test.array}}}
			err := ValidateJobs(queue)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateJobs error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateJobsRejectsExpandedJobIDCollision(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "array", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2}},
		{ID: "array-1", Command: []string{"true"}},
	}}
	if err := ValidateJobs(queue); err == nil || !strings.Contains(err.Error(), `duplicate job ID "array-1"`) {
		t.Fatalf("ValidateJobs error = %v", err)
	}
}

func TestValidateJobsRejectsInvalidJobFields(t *testing.T) {
	tests := []struct {
		name string
		job  model.QueuedCommand
		want string
	}{
		{name: "empty command", job: model.QueuedCommand{ID: "job-1"}, want: `job "job-1" has an empty command`},
		{name: "empty executable", job: model.QueuedCommand{ID: "job-1", Command: []string{""}}, want: `job "job-1" has an empty command`},
		{name: "invalid ID", job: model.QueuedCommand{ID: "../job-1", Command: []string{"true"}}, want: `invalid job ID "../job-1"`},
		{name: "invalid environment name", job: model.QueuedCommand{ID: "job-1", Command: []string{"true"}, Environment: []string{"BAD-NAME=value"}}, want: `job "job-1" has invalid environment`},
		{name: "NUL environment value", job: model.QueuedCommand{ID: "job-1", Command: []string{"true"}, Environment: []string{"KEY=value\x00tail"}}, want: `job "job-1" has invalid environment`},
		{name: "NUL working directory", job: model.QueuedCommand{ID: "job-1", Command: []string{"true"}, WorkingDirectory: "work\x00dir"}, want: `job "job-1" working directory contains a NUL byte`},
		{name: "NUL command argument", job: model.QueuedCommand{ID: "job-1", Command: []string{"printf", "value\x00tail"}}, want: `job "job-1" command contains a NUL byte`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateJobs(model.Queue{Commands: []model.QueuedCommand{test.job}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateJobs error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateJobsRejectsDuplicateJobID(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "job-1", Command: []string{"true"}},
		{ID: "job-1", Command: []string{"true"}},
	}}
	if err := ValidateJobs(queue); err == nil || !strings.Contains(err.Error(), `duplicate job ID "job-1"`) {
		t.Fatalf("ValidateJobs error = %v", err)
	}
}
