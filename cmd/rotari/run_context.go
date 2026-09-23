package main

import (
	"os"
	"path/filepath"
	"time"

	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeRunContext(paths pathSet, runID, cwd string) error {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	context := captureRunContext(cwd)
	if configPath := effectiveConfigPath(paths.BaseDir, paths.ProjectName); configPath != "" {
		context.ConfigPaths = []string{configPath}
	}
	if context.StartedLoad != nil {
		if err := state.AppendLoadSample(loadSamplesPath(paths, runID), LoadSample{At: nowRFC3339Nano(), LoadAverage: *context.StartedLoad}); err != nil {
			return err
		}
	}
	return state.SaveContext(jsonStore(), runDir, context)
}

func finishRunContext(paths pathSet, runID string) error {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	context := RunContext{}
	if loaded, err := state.LoadContext(jsonStore(), runDir); err == nil {
		context = loaded
	}
	context.FinishedLoad = runcontract.ReadLoadAverage()
	if context.FinishedLoad != nil {
		if err := state.AppendLoadSample(loadSamplesPath(paths, runID), LoadSample{At: nowRFC3339Nano(), LoadAverage: *context.FinishedLoad}); err != nil {
			return err
		}
	}
	return state.SaveContext(jsonStore(), runDir, context)
}

func captureRunContext(cwd string) RunContext {
	hostname, _ := os.Hostname()
	return RunContext{CWD: cwd, Hostname: hostname, StartedLoad: runcontract.ReadLoadAverage()}
}

const loadSampleInterval = 10 * time.Second

func startRunLoadSampling(paths pathSet, runID string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(loadSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = appendRunLoadSample(paths, runID)
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func loadSamplesPath(paths pathSet, runID string) string {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return ""
	}
	return filepath.Join(runDir, "load_samples.jsonl")
}

func appendRunLoadSample(paths pathSet, runID string) error {
	load := runcontract.ReadLoadAverage()
	if load == nil {
		return nil
	}
	return state.AppendLoadSample(loadSamplesPath(paths, runID), LoadSample{At: nowRFC3339Nano(), LoadAverage: *load})
}
