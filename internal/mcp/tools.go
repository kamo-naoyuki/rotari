package mcp

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/report"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// The tools here present the shared read-side functions that `rotari show`
// and `rotari lineage` use. They add no selection, status, or grouping rules
// of their own, and they return no absolute paths: a basedir is named by
// its registry reference and its last path element.

type ListProjectsInput struct{}

type ListProjectsOutput struct {
	Projects []ProjectOverview `json:"projects"`
	// Unreadable lists registered basedirs whose projects could not be read.
	Unreadable []UnreadableBaseDir `json:"unreadable,omitempty"`
}

type ProjectOverview struct {
	BaseDirRef  string `json:"basedir_ref" jsonschema:"path-free reference to the project's state directory"`
	BaseDirName string `json:"basedir_name" jsonschema:"last element of the state directory's path"`
	Project     string `json:"project"`
	State       string `json:"state" jsonschema:"idle, running, or interrupted"`
	Queued      int    `json:"queued"`
	Runs        int    `json:"runs"`
	LastRunID   string `json:"last_run_id,omitempty"`
	LastStatus  string `json:"last_run_status,omitempty"`
	LastJobs    int    `json:"last_run_jobs,omitempty"`
	LastFailed  int    `json:"last_run_failed,omitempty"`
}

type UnreadableBaseDir struct {
	BaseDirRef  string `json:"basedir_ref"`
	BaseDirName string `json:"basedir_name"`
	Error       string `json:"error"`
}

// listProjects lists the projects of every basedir registered under
// masterDir, as `rotari show` does, without writing.
func listProjects(masterDir string) (ListProjectsOutput, error) {
	baseDirs, _, err := basedirregistry.Discover(masterDir)
	if err != nil {
		return ListProjectsOutput{}, err
	}
	output := ListProjectsOutput{Projects: []ProjectOverview{}}
	for _, baseDir := range baseDirs {
		overviews, err := project.Overviews(baseDir, false)
		if err != nil {
			output.Unreadable = append(output.Unreadable, UnreadableBaseDir{
				BaseDirRef: basedirregistry.Ref(baseDir), BaseDirName: filepath.Base(baseDir),
				Error: strings.ReplaceAll(err.Error(), baseDir, "BASEDIR"),
			})
			continue
		}
		for _, overview := range overviews {
			output.Projects = append(output.Projects, ProjectOverview{
				BaseDirRef: basedirregistry.Ref(baseDir), BaseDirName: filepath.Base(baseDir), Project: overview.Name,
				State: stateName(overview.State), Queued: overview.Queued, Runs: overview.Runs,
				LastRunID: overview.LastRun.ID, LastStatus: overview.LastRun.Status, LastJobs: overview.LastRun.Jobs, LastFailed: overview.LastRun.Failed,
			})
		}
	}
	return output, nil
}

func stateName(runState project.RunState) string {
	switch runState {
	case project.Running:
		return "running"
	case project.Interrupted:
		return "interrupted"
	}
	return "idle"
}

type RunSummaryInput struct {
	RunID string `json:"run_id" jsonschema:"exact run ID"`
}

type RunSummaryOutput struct {
	BaseDirRef string                `json:"basedir_ref"`
	Project    string                `json:"project"`
	Summary    runlineage.RunSummary `json:"summary"`
}

// runSummary describes one run as `rotari lineage RUN_ID` does: counts and
// failures grouped by cause, with evidence lines redacted by pattern.
func runSummary(masterDir string, input RunSummaryInput) (RunSummaryOutput, error) {
	location, paths, err := registeredRun(masterDir, input.RunID)
	if err != nil {
		return RunSummaryOutput{}, err
	}
	summary, err := runview.Summary(paths, location.RunID, state.NewStore(state.DirectoryMode(), state.FileMode()))
	if err != nil {
		return RunSummaryOutput{}, err
	}
	for index := range summary.Failures {
		summary.Failures[index].Example.Evidence = report.RedactPatterns(summary.Failures[index].Example.Evidence)
	}
	return RunSummaryOutput{BaseDirRef: basedirregistry.Ref(location.BaseDir), Project: location.ProjectName, Summary: summary}, nil
}

type CompareRunsInput struct {
	RunID         string `json:"run_id" jsonschema:"exact ID of the newer run"`
	PreviousRunID string `json:"previous_run_id,omitempty" jsonschema:"exact ID of the older run of the same project; defaults to the run that started just before run_id"`
}

type CompareRunsOutput struct {
	BaseDirRef string            `json:"basedir_ref"`
	Project    string            `json:"project"`
	Comparison runlineage.Result `json:"comparison"`
}

// compareRuns compares two runs of one project as
// `rotari lineage PREVIOUS_RUN_ID RUN_ID` does.
func compareRuns(masterDir string, input CompareRunsInput) (CompareRunsOutput, error) {
	location, paths, err := registeredRun(masterDir, input.RunID)
	if err != nil {
		return CompareRunsOutput{}, err
	}
	previousID := input.PreviousRunID
	if previousID == "" {
		if previousID, err = runview.PreviousRun(paths, location.RunID); err != nil {
			return CompareRunsOutput{}, err
		}
	} else {
		previous, _, err := registeredRun(masterDir, previousID)
		if err != nil {
			return CompareRunsOutput{}, err
		}
		if previous.BaseDir != location.BaseDir || previous.ProjectName != location.ProjectName {
			return CompareRunsOutput{}, fmt.Errorf("runs %q and %q belong to different projects", previousID, location.RunID)
		}
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	from, err := runview.LoadRun(paths, previousID, store)
	if err != nil {
		return CompareRunsOutput{}, err
	}
	to, err := runview.LoadRun(paths, location.RunID, store)
	if err != nil {
		return CompareRunsOutput{}, err
	}
	return CompareRunsOutput{BaseDirRef: basedirregistry.Ref(location.BaseDir), Project: location.ProjectName, Comparison: runlineage.Compare(from, to)}, nil
}

// registeredRun locates runID through masterDir's run registry.
func registeredRun(masterDir, runID string) (runregistry.Location, state.ProjectPaths, error) {
	location, err := resolve.RegisteredRun(masterDir, runID)
	if err != nil {
		return runregistry.Location{}, state.ProjectPaths{}, err
	}
	paths, err := state.ResolveProjectPaths(location.BaseDir, location.ProjectName)
	if err != nil {
		return runregistry.Location{}, state.ProjectPaths{}, err
	}
	return location, paths, nil
}
