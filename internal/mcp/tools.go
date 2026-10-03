package mcp

import (
	"fmt"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
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
				Error: hideBaseDir(err.Error(), baseDir),
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

// failureMemberLimit is how many jobs of a failure group the summary lists
// unless all_jobs is set.
const failureMemberLimit = 10

type RunSummaryInput struct {
	RunID   string `json:"run_id" jsonschema:"exact run ID"`
	AllJobs bool   `json:"all_jobs,omitempty" jsonschema:"list every job of each failure group; by default the first 10, with the rest counted in jobs_omitted"`
}

type RunSummaryOutput struct {
	BaseDirRef string                `json:"basedir_ref"`
	Project    string                `json:"project"`
	State      project.RunPhase      `json:"state" jsonschema:"running, interrupted (its supervisor stopped while it ran), finished, or ended (stopped without a summary); counts change only while running"`
	Summary    runlineage.RunSummary `json:"summary"`
}

// runSummary describes one run as `rotari lineage RUN_ID` does: counts and
// failures grouped by cause, with evidence lines redacted by pattern, and
// whether the run is still running, as `rotari wait` decides it.
func runSummary(masterDir string, input RunSummaryInput) (RunSummaryOutput, error) {
	location, paths, err := registeredRun(masterDir, input.RunID)
	if err != nil {
		return RunSummaryOutput{}, err
	}
	phase, err := project.RunPhaseOf(paths, location.RunID)
	if err != nil {
		return RunSummaryOutput{}, err
	}
	output := RunSummaryOutput{BaseDirRef: basedirregistry.Ref(location.BaseDir), Project: location.ProjectName, State: phase}
	// A run that has just started has no jobs yet; runview.LoadRun says so.
	summary, err := runview.Summary(paths, location.RunID, state.NewStore(state.DirectoryMode(), state.FileMode()))
	if err != nil {
		return RunSummaryOutput{}, err
	}
	for index := range summary.Failures {
		summary.Failures[index].Example.Evidence = report.RedactPatterns(summary.Failures[index].Example.Evidence)
	}
	if !input.AllJobs {
		runlineage.LimitMembers(summary.Failures, failureMemberLimit)
	}
	output.Summary = summary
	return output, nil
}

type CompareRunsInput struct {
	RunID         string `json:"run_id" jsonschema:"exact ID of the newer run"`
	PreviousRunID string `json:"previous_run_id,omitempty" jsonschema:"exact ID of the older run of the same project; defaults to the run that started just before run_id"`
	AllJobs       bool   `json:"all_jobs,omitempty" jsonschema:"list every job; by default jobs whose result and definition did not change are only counted in hidden_unchanged, and beyond 20 listed jobs those whose only change is their definition are counted in hidden_changed"`
}

// comparisonJobLimit is how many changed jobs a comparison lists unless
// all_jobs is set.
const comparisonJobLimit = 20

type CompareRunsOutput struct {
	BaseDirRef string            `json:"basedir_ref"`
	Project    string            `json:"project"`
	Comparison runlineage.Result `json:"comparison"`
	// HiddenUnchanged counts the unchanged jobs left out of Comparison.Jobs.
	HiddenUnchanged int `json:"hidden_unchanged,omitempty"`
	// HiddenChanged counts the jobs left out beyond the limit, whose only
	// change is their definition.
	HiddenChanged int `json:"hidden_changed,omitempty"`
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
	output := CompareRunsOutput{BaseDirRef: basedirregistry.Ref(location.BaseDir), Project: location.ProjectName, Comparison: runlineage.Compare(from, to)}
	if !input.AllJobs {
		shown := output.Comparison.Jobs[:0]
		for _, job := range output.Comparison.Jobs {
			if job.Notable() {
				shown = append(shown, job)
			}
		}
		output.HiddenUnchanged = len(output.Comparison.Jobs) - len(shown)
		output.Comparison.Jobs, output.HiddenChanged = limitComparedJobs(shown, comparisonJobLimit)
	}
	return output, nil
}

// limitComparedJobs keeps at most limit jobs in their order. Jobs whose
// result changed are kept first; jobs whose only change is their definition
// fill the rest, and the ones left out are counted.
func limitComparedJobs(jobs []runlineage.JobDiff, limit int) ([]runlineage.JobDiff, int) {
	if len(jobs) <= limit {
		return jobs, 0
	}
	keep := make([]bool, len(jobs))
	kept := 0
	for _, resultChanged := range []bool{true, false} {
		for index, job := range jobs {
			if kept < limit && !keep[index] && (job.Transition != runlineage.TransitionUnchanged) == resultChanged {
				keep[index] = true
				kept++
			}
		}
	}
	shown := make([]runlineage.JobDiff, 0, kept)
	for index, job := range jobs {
		if keep[index] {
			shown = append(shown, job)
		}
	}
	return shown, len(jobs) - kept
}

type CheckProjectInput struct {
	BaseDirRef string `json:"basedir_ref" jsonschema:"basedir_ref of the project, from rotari_list_projects"`
	Project    string `json:"project" jsonschema:"project name"`
}

type CheckProjectOutput struct {
	BaseDirRef string `json:"basedir_ref"`
	Project    string `json:"project"`
	State      string `json:"state" jsonschema:"ready, empty, running, locked (running on another host), or interrupted"`
	Runnable   bool   `json:"runnable"`
	Queued     *int   `json:"queued" jsonschema:"queued jobs; null when the queue of an active or interrupted project cannot be read"`
	Lock       string `json:"lock"`
	RunID      string `json:"run_id,omitempty"`
	Revision   string `json:"revision"`
}

// checkProject reports whether a project's queued run can start, as
// `rotari check` does without --deep, which needs the jobs' host.
func checkProject(masterDir string, input CheckProjectInput) (CheckProjectOutput, error) {
	baseDir, err := basedirregistry.Find(masterDir, input.BaseDirRef)
	if err != nil {
		return CheckProjectOutput{}, err
	}
	if err := resolve.RequireProject(baseDir, input.Project); err != nil {
		return CheckProjectOutput{}, err
	}
	paths, err := state.ResolveProjectPaths(baseDir, input.Project)
	if err != nil {
		return CheckProjectOutput{}, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	runner := projectrun.Runner{Store: store, Executors: executor.NewRegistry(store, func(string, ...any) {})}
	result, err := runner.Check(paths, nil)
	if err != nil {
		return CheckProjectOutput{}, err
	}
	output := CheckProjectOutput{BaseDirRef: input.BaseDirRef, Project: input.Project, State: result.State, Runnable: result.Runnable, Lock: result.Lock, RunID: result.RunID, Revision: result.Revision}
	if result.QueuedKnown {
		output.Queued = &result.Queued
	}
	return output, nil
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
