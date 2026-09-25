package main

import (
	"reflect"
	"testing"

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
			PartialArray: partialArray,
		}
		args := runcontract.WorkerArgs(options, true, executorRunSettingNames)
		parsed, err := parseWorkerRunArgs(args[1:])
		if err != nil {
			t.Fatalf("partialArray=%v: parseWorkerRunArgs(%q) returned error: %v", partialArray, args, err)
		}
		parsed.ExecutorSettings = nil
		if !reflect.DeepEqual(parsed, options) {
			t.Fatalf("partialArray=%v: parsed options = %#v, want %#v", partialArray, parsed, options)
		}
	}
}
