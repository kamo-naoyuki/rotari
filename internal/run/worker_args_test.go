package run

import (
	"reflect"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
)

func TestWorkerArgsKeepsBaseDirSeparateFromCWD(t *testing.T) {
	args := WorkerArgs(Options{
		BaseDir: "/state", QueueName: "demo", RunID: "run-1", RunName: "nightly",
		LocalConcurrency: 2, BatchMaxActive: 3, Retry: 1, CWD: "/work", PartialArray: true,
		ExecutorSettings: map[string]executor.RunSettings{"slurm": {Concurrency: 4, Options: []string{"--partition short"}, SubmitInterval: 250 * time.Millisecond, SubmitRetryLimit: 3}},
	}, true, []string{"slurm"})
	want := []string{"__worker-run", "--basedir", "/state", "--slurm-concurrency", "4", "--slurm-options", "--partition short", "--slurm-submit-interval", "250ms", "--slurm-submit-retry-limit", "3", "--partial-array", "true", "demo", "run-1", "nightly", "2", "3", "1", "/work"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("WorkerArgs() = %#v, want %#v", args, want)
	}
}
