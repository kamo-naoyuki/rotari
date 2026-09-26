package projectrun

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestValidateQueueRejectsInvalidExecutorConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		queue model.Queue
		want  string
	}{
		{
			name:  "unknown default executor",
			queue: model.Queue{DefaultExecutor: "unknown", Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}},
			want:  "unsupported executor: unknown",
		},
		{
			name:  "unknown job executor",
			queue: model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Executor: "unknown", Command: []string{"true"}}}},
			want:  `job "job-1" uses unsupported executor: unknown`,
		},
		{
			name:  "invalid scheduler option quoting",
			queue: model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Executor: "slurm", ExecutorOptions: []string{`"unterminated`}, Command: []string{"true"}}}},
			want:  `job "job-1" executor options: invalid executor option`,
		},
		{
			name:  "missing SSH target",
			queue: model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Executor: "ssh", Command: []string{"true"}}}},
			want:  `job "job-1" executor options: SSH executor requires its first executor option to be the target host`,
		},
		{
			name:  "reserved Slurm array option",
			queue: model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Executor: "slurm", ExecutorOptions: []string{"--array=1-2"}, Array: &model.ArraySpec{First: 1, Last: 2}, Command: []string{"true"}}}},
			want:  `job "job-1" executor options: executor options must not include --array`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner, _ := testRunner(t)
			err := runner.ValidateQueue(test.queue, "", nil, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateQueue error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateQueueUsesDispatchOptionPriority(t *testing.T) {
	queue := model.Queue{
		DefaultExecutorOptions: []string{"invalid-default"},
		Commands: []model.QueuedCommand{{
			ID:              "job-1",
			Executor:        "ssh",
			ExecutorOptions: []string{"worker.example"},
			Command:         []string{"true"},
		}},
	}
	settings := executor.RunSettingsMap{"ssh": {Options: []string{"invalid-setting"}}}
	runner, _ := testRunner(t)
	if err := runner.ValidateQueue(queue, "", []string{"invalid-common"}, settings); err != nil {
		t.Fatalf("ValidateQueue rejected job-specific SSH target: %v", err)
	}
}
