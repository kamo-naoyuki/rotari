package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Overview is what a project list shows of one project.
type Overview struct {
	BaseDir string
	Name    string
	Queued  int
	Runs    int
	State   RunState
	LastRun LastRun
}

// LastRun describes a project's last run. Status is the run summary's, or
// empty without a readable summary; Jobs and Failed count its results.
type LastRun struct {
	ID     string
	Status string
	Jobs   int
	Failed int
}

// Overviews lists the projects of baseDir. With cleanupStale, a stale local
// lock is removed while inspecting, as Inspect does.
func Overviews(baseDir string, cleanupStale bool) ([]Overview, error) {
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read projects directory %q: %w", baseDir, err)
	}
	overviews := make([]Overview, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !state.IsValidPathElement(entry.Name()) {
			continue
		}
		paths, err := state.ResolveProjectPaths(baseDir, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to resolve project %q: %w", entry.Name(), err)
		}
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load queue for project %q: %w", entry.Name(), err)
		}
		inspection, err := Inspect(paths, cleanupStale)
		if err != nil {
			return nil, fmt.Errorf("failed to check project %q state: %w", entry.Name(), err)
		}
		meta, err := state.LoadMeta(paths.MetaFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load project %q metadata: %w", entry.Name(), err)
		}
		overviews = append(overviews, Overview{
			BaseDir: baseDir, Name: entry.Name(), Queued: len(queue.Commands), Runs: CountRuns(paths),
			State: inspection.State, LastRun: lastRun(paths, meta.LastRunID),
		})
	}
	return overviews, nil
}

// CountRuns counts a project's saved run directories.
func CountRuns(paths state.ProjectPaths) int {
	entries, err := os.ReadDir(paths.RunsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

// lastRun reads runID's summary counts; a missing or unreadable summary
// leaves Status empty.
func lastRun(paths state.ProjectPaths, runID string) LastRun {
	last := LastRun{ID: runID}
	if runID == "" {
		return last
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return last
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return last
	}
	succeeded, failed := model.CountRunResults(summary.Results)
	last.Status, last.Jobs, last.Failed = summary.Status, succeeded+failed, failed
	return last
}
