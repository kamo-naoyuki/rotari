package main

import (
	"fmt"
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
	context.ConfigPaths = configPathsForRun(paths.BaseDir, paths.ProjectName)
	if len(context.ConfigPaths) > 0 {
		files, snapshotPaths, err := snapshotRunConfigs(runDir, context.ConfigPaths)
		if err != nil {
			return err
		}
		context.ConfigSnapshotFiles = files
		context.ConfigSnapshotPaths = snapshotPaths
	}
	if context.StartedLoad != nil {
		if err := state.AppendLoadSample(loadSamplesPath(paths, runID), LoadSample{At: nowRFC3339Nano(), LoadAverage: *context.StartedLoad}); err != nil {
			return err
		}
	}
	return state.SaveContext(jsonStore(), runDir, context)
}

func snapshotRunConfigs(runDir string, configPaths []string) ([]string, []string, error) {
	snapshotDir, err := state.SafeJoin(runDir, "configs")
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(snapshotDir, stateDirMode()); err != nil {
		return nil, nil, err
	}
	files := make([]string, 0, len(configPaths))
	snapshotPaths := make([]string, 0, len(configPaths))
	for index, configPath := range configPaths {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, nil, err
		}
		fileName := fmt.Sprintf("config-%d%s", index+1, filepath.Ext(configPath))
		snapshotPath, err := state.SafeJoin(snapshotDir, fileName)
		if err != nil {
			return nil, nil, err
		}
		if err := os.WriteFile(snapshotPath, data, stateFileMode()); err != nil {
			return nil, nil, err
		}
		files = append(files, fileName)
		snapshotPaths = append(snapshotPaths, snapshotPath)
	}
	return files, snapshotPaths, nil
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
