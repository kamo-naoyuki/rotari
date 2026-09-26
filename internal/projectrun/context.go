package projectrun

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// LoadSampleInterval is how often a running run appends a load sample.
const LoadSampleInterval = 10 * time.Second

// WriteContext records where and how a run starts: working directory, host,
// load, and a snapshot of every active config file.
func (runner Runner) WriteContext(paths state.ProjectPaths, runID, cwd string) error {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	context := model.RunContext{CWD: cwd, Hostname: hostname, StartedLoad: run.ReadLoadAverage()}
	if runner.ConfigPaths != nil {
		context.ConfigPaths = runner.ConfigPaths(paths)
	}
	if len(context.ConfigPaths) > 0 {
		files, snapshotPaths, err := snapshotRunConfigs(runDir, context.ConfigPaths)
		if err != nil {
			return err
		}
		context.ConfigSnapshotFiles = files
		context.ConfigSnapshotPaths = snapshotPaths
	}
	if context.StartedLoad != nil {
		if err := runner.appendLoadSample(runDir, *context.StartedLoad); err != nil {
			return err
		}
	}
	return state.SaveContext(runner.Store, runDir, context)
}

// FinishContext adds the load at the end of a run to its context.
func (runner Runner) FinishContext(paths state.ProjectPaths, runID string) error {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	context := model.RunContext{}
	if loaded, err := state.LoadContext(runner.Store, runDir); err == nil {
		context = loaded
	}
	context.FinishedLoad = run.ReadLoadAverage()
	if context.FinishedLoad != nil {
		if err := runner.appendLoadSample(runDir, *context.FinishedLoad); err != nil {
			return err
		}
	}
	return state.SaveContext(runner.Store, runDir, context)
}

func snapshotRunConfigs(runDir string, configPaths []string) ([]string, []string, error) {
	snapshotDir, err := state.SafeJoin(runDir, "configs")
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(snapshotDir, state.DirectoryMode()); err != nil {
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
		if err := os.WriteFile(snapshotPath, data, state.FileMode()); err != nil {
			return nil, nil, err
		}
		files = append(files, fileName)
		snapshotPaths = append(snapshotPaths, snapshotPath)
	}
	return files, snapshotPaths, nil
}

// startLoadSampling appends a load sample every LoadSampleInterval until the
// returned function is called.
func (runner Runner) startLoadSampling(paths state.ProjectPaths, runID string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	go func() {
		defer close(done)
		ticker := time.NewTicker(LoadSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if load := run.ReadLoadAverage(); load != nil && err == nil {
					_ = runner.appendLoadSample(runDir, *load)
				}
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

func (runner Runner) appendLoadSample(runDir string, load model.LoadAverage) error {
	return state.AppendLoadSample(filepath.Join(runDir, state.LoadSamplesFileName), model.LoadSample{At: runner.timestampNano(), LoadAverage: load})
}
