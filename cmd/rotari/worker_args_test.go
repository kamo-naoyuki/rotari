package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
)

// TestWorkerArgsParseBackToOptions checks the launcher and the worker agree on
// the argument format; a mismatch leaves every async run interrupted.
func TestWorkerArgsParseBackToOptions(t *testing.T) {
	for _, partialArray := range []bool{true, false} {
		options := runcontract.Options{
			BaseDir: "/state", QueueName: "demo", RunID: "run-1", RunName: "nightly",
			LocalConcurrency: 2, BatchMaxActive: 3, Retry: 1, CWD: "/work",
			Executor: "slurm", ExecutorOptions: []string{"--partition=short"},
			Selection: "failed", JobIDs: []string{"job-1", "job-2"}, SourceRunID: "run-0",
			Scope:        model.CommandSelector{Stage: "train", Matrix: "sweep"},
			PartialArray: partialArray,
			ExecutorSettings: map[string]executor.RunSettings{
				"slurm": {Concurrency: 4, Options: []string{"--partition short"}, SubmitInterval: 250 * time.Millisecond, SubmitRetryLimit: 3},
			},
		}
		args := runcontract.WorkerArgs(options, true, executorRunSettingNames)
		parsed, err := parseWorkerRunArgs(args[1:])
		if err != nil {
			t.Fatalf("partialArray=%v: parseWorkerRunArgs(%q) returned error: %v", partialArray, args, err)
		}
		for name, setting := range parsed.ExecutorSettings {
			if name != "slurm" && !reflect.DeepEqual(setting, executor.RunSettings{Options: stringSliceFlag(nil)}) {
				t.Fatalf("partialArray=%v: unexpected %s settings %#v", partialArray, name, setting)
			}
		}
		parsed.ExecutorSettings = map[string]executor.RunSettings{"slurm": parsed.ExecutorSettings["slurm"]}
		if !reflect.DeepEqual(parsed, options) {
			t.Fatalf("partialArray=%v: parsed options = %#v, want %#v", partialArray, parsed, options)
		}
	}
}
