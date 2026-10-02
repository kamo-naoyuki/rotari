package runview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// RunsByStart lists a project's run IDs oldest first. Runs of one project
// never overlap, so start order is run order. Run IDs only have one-second
// resolution, so runs are ordered by their first load sample, which has
// nanoseconds, then by the summary's start time, then by run ID.
func RunsByStart(paths state.ProjectPaths) ([]string, error) {
	entries, err := os.ReadDir(paths.RunsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read runs: %w", err)
	}
	type startedRun struct {
		id      string
		started time.Time
	}
	runs := make([]startedRun, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			runs = append(runs, startedRun{id: entry.Name(), started: runStartTime(paths, entry.Name())})
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		if !runs[i].started.Equal(runs[j].started) {
			return runs[i].started.Before(runs[j].started)
		}
		return runs[i].id < runs[j].id
	})
	runIDs := make([]string, len(runs))
	for index, run := range runs {
		runIDs[index] = run.id
	}
	return runIDs, nil
}

// PreviousRun returns the run of the project that started just before runID.
func PreviousRun(paths state.ProjectPaths, runID string) (string, error) {
	runIDs, err := RunsByStart(paths)
	if err != nil {
		return "", err
	}
	for index, candidate := range runIDs {
		if candidate == runID {
			if index == 0 {
				return "", fmt.Errorf("run %s has no earlier run in project %q to compare with", runID, paths.ProjectName)
			}
			return runIDs[index-1], nil
		}
	}
	return "", fmt.Errorf("run %q not found", runID)
}

// Summary loads runID and describes it without comparing it to another run:
// counts, diagnosis counts, failure groups, and origins.
func Summary(paths state.ProjectPaths, runID string, store state.Store) (runlineage.RunSummary, error) {
	run, err := LoadRun(paths, runID, store)
	if err != nil {
		return runlineage.RunSummary{}, err
	}
	return runlineage.RunSummary{
		Run:       runlineage.RunInfo{ID: run.ID, Name: run.Name},
		Counts:    runlineage.Summarize(run),
		Diagnoses: runlineage.SummarizeDiagnoses(run),
		Failures:  runlineage.FailureGroups(run),
		Origins:   runlineage.SummarizeOrigins(run),
	}, nil
}

// runStartTime returns when a run started, or the zero time when unknown.
func runStartTime(paths state.ProjectPaths, runID string) time.Time {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return time.Time{}
	}
	if samples := state.ReadLoadSamples(filepath.Join(runDir, state.LoadSamplesFileName)); len(samples) > 0 {
		if started, err := time.Parse(time.RFC3339Nano, samples[0].At); err == nil {
			return started
		}
	}
	if summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		if started, err := time.Parse(time.RFC3339, summary.StartedAt); err == nil {
			return started
		}
	}
	return time.Time{}
}
