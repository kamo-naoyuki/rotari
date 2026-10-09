package projectrun

import (
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// LoadSampleInterval is how often a running run appends a load sample.
const LoadSampleInterval = 10 * time.Second

// WriteContext records where and how a run starts: working directory, host,
// the launching process, load, and snapshots of the loaded command and
// notification configs.
func (runner Runner) WriteContext(paths state.ProjectPaths, runID, cwd, configPath string, origin *model.LaunchOrigin, snapshots ...*model.FileConfigSnapshot) error {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	context := model.RunContext{CWD: cwd, Hostname: hostname, LaunchOrigin: origin, StartedLoad: readLoadAverage()}
	var snapshot *model.FileConfigSnapshot
	if len(snapshots) > 0 {
		snapshot = snapshots[0]
	}
	if snapshot != nil {
		snapshotDir := filepath.Join(runDir, "configs")
		if err := os.MkdirAll(snapshotDir, state.DirectoryMode()); err != nil {
			return err
		}
		path := filepath.Join(snapshotDir, "config.toml")
		if err := os.WriteFile(path, []byte(snapshot.Content), state.FileMode()); err != nil {
			return err
		}
		context.ConfigSources = append([]model.ConfigSource(nil), snapshot.Sources...)
		for _, source := range snapshot.Sources {
			context.ConfigPaths = append(context.ConfigPaths, source.Path)
		}
		context.ConfigSnapshotFiles = []string{"config.toml"}
		context.ConfigSnapshotPaths = []string{path}
		// Only the separate notification file may be loaded here. Ordinary
		// sources have already been captured; do not reread any of them.
		configPath = ""
	}
	var additionalPaths []string
	if runner.ConfigPaths != nil {
		additionalPaths = runner.ConfigPaths(paths, configPath)
	}
	if snapshot != nil {
		additionalPaths = notificationPaths(additionalPaths)
	}
	context.ConfigPaths = append(context.ConfigPaths, additionalPaths...)
	if len(additionalPaths) > 0 {
		files, snapshotPaths, err := snapshotRunConfigs(runDir, additionalPaths)
		if err != nil {
			return err
		}
		context.ConfigSnapshotFiles = append(context.ConfigSnapshotFiles, files...)
		context.ConfigSnapshotPaths = append(context.ConfigSnapshotPaths, snapshotPaths...)
	}
	if context.StartedLoad != nil {
		if err := runner.appendLoadSample(runDir, *context.StartedLoad); err != nil {
			return err
		}
	}
	return state.SaveContext(runner.Store, runDir, context)
}

func notificationPaths(paths []string) []string {
	var notifications []string
	for _, path := range paths {
		if filepath.Base(path) == "notifications.toml" {
			notifications = append(notifications, path)
		}
	}
	return notifications
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
	context.FinishedLoad = readLoadAverage()
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
	for _, configPath := range configPaths {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, nil, err
		}
		fileName := filepath.Base(configPath)
		if fileName != "notifications.toml" {
			fileName = "config" + filepath.Ext(configPath)
		}
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
				if load := readLoadAverage(); load != nil && err == nil {
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
