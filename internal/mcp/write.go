package mcp

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
	"github.com/kamo-naoyuki/rotari/internal/workflowstate"
)

// The tools here change a project. Each comes as a read-only preview, which
// returns the project revision, and a write that applies only at a revision,
// so an MCP client can ask its user before each write while previews run
// freely. They use the shared operations `rotari import`, `run`, and `retry`
// use, under the same project.Guard.

// Options supplies what the tools that change state need from the rotari
// process that serves them.
type Options struct {
	// NewJobID returns a fresh job ID for an imported queue.
	NewJobID func() string
	// StartRun starts a supervisor for paths' project and sends it request,
	// returning its response. Without it, runs cannot be started.
	StartRun func(paths state.ProjectPaths, request server.Request) (server.Response, error)
}

type ImportInput struct {
	BaseDirRef string `json:"basedir_ref" jsonschema:"basedir_ref of the destination state directory, from rotari_list_projects"`
	Project    string `json:"project" jsonschema:"destination project; created if it does not exist"`
	Manifest   string `json:"manifest" jsonschema:"workflow manifest text, as rotari export writes it"`
	Format     string `json:"format,omitempty" jsonschema:"manifest format: yaml (default), json, or toml"`
	Overwrite  bool   `json:"overwrite,omitempty" jsonschema:"replace a queue that already has jobs"`
	Detail     bool   `json:"detail,omitempty" jsonschema:"also return the full plan, with each job's command and each task's source attempt"`
}

// ImportOutput is an import plan's summary and, when asked for, the plan.
type ImportOutput struct {
	workflowstate.PlanSummary
	Plan *workflowstate.Plan `json:"plan,omitempty"`
}

type ApplyImportInput struct {
	ImportInput
	IfRevision string `json:"if_revision" jsonschema:"revision from rotari_preview_import; the import applies only if the project is still at it"`
}

type RunInput struct {
	BaseDirRef string `json:"basedir_ref" jsonschema:"basedir_ref of the project, from rotari_list_projects"`
	Project    string `json:"project" jsonschema:"project name"`
	RunID      string `json:"run_id,omitempty" jsonschema:"settled source run ID, or latest; build the new run from it without changing the next queue"`
	Retry      bool   `json:"retry,omitempty" jsonschema:"run only failed and unfinished jobs and carry other results, as rotari retry does: use a non-empty next queue as-is, or build a run snapshot from the last run when the queue is empty"`
}

type StartRunInput struct {
	RunInput
	IfRevision string `json:"if_revision" jsonschema:"revision from rotari_preview_run; the run starts only if the project is still at it"`
	RunName    string `json:"run_name,omitempty" jsonschema:"label for the run"`
}

type PlannedJob struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// DependsOnRerun names the dependency that executes, for a job the
	// selection left out but that executes because of it.
	DependsOnRerun string `json:"depends_on_rerun,omitempty" jsonschema:"for a job that executes only because a job it depends on executes: that dependency"`
}

type RunPreviewOutput struct {
	Project           string       `json:"project"`
	Execute           []PlannedJob `json:"execute" jsonschema:"jobs the run would execute"`
	Jobs              int          `json:"jobs" jsonschema:"jobs in the run snapshot"`
	Carried           int          `json:"carried" jsonschema:"results the run would carry forward instead of executing"`
	SourceRun         string       `json:"source_run,omitempty" jsonschema:"saved run whose results the selection references"`
	OmittedSourceJobs []string     `json:"omitted_source_jobs,omitempty" jsonschema:"failed or unfinished jobs in the latest run not represented in a non-empty queue"`
	Revision          string       `json:"revision" jsonschema:"revision to pass to rotari_start_run"`
}

type StartRunOutput struct {
	Project           string   `json:"project"`
	RunID             string   `json:"run_id"`
	SourceRun         string   `json:"source_run,omitempty"`
	OmittedSourceJobs []string `json:"omitted_source_jobs,omitempty"`
}

// projectPaths resolves a project named by basedir_ref and name.
func projectPaths(masterDir, baseDirRef, projectName string) (string, state.ProjectPaths, error) {
	baseDir, err := basedirregistry.Find(masterDir, baseDirRef)
	if err != nil {
		return "", state.ProjectPaths{}, err
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	return baseDir, paths, err
}

func (tools writeTools) runner() projectrun.Runner {
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	// MCP run planning has no user-facing scheduler log stream; tool responses
	// carry the plan and run ID, so executor diagnostics remain in run state.
	quietLogger := func(string, ...any) {}
	return projectrun.Runner{Store: store, Executors: executor.NewRegistry(store, quietLogger), NewJobID: tools.options.NewJobID}
}

type writeTools struct {
	masterDir string
	options   Options
}

// importManifest previews or applies an import as `rotari import` does and
// returns the plan's summary, with the plan itself when input.Detail is set.
func (tools writeTools) importManifest(input ImportInput, guard project.Guard) (ImportOutput, error) {
	plan, err := tools.importPlan(input, guard)
	if err != nil {
		return ImportOutput{}, err
	}
	output := ImportOutput{PlanSummary: plan.Summary()}
	if input.Detail {
		output.Plan = &plan
	}
	return output, nil
}

func (tools writeTools) importPlan(input ImportInput, guard project.Guard) (workflowstate.Plan, error) {
	baseDir, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return workflowstate.Plan{}, err
	}
	if containsRedaction(input.Manifest) {
		return workflowstate.Plan{}, errors.New("the manifest contains redacted placeholders from rotari_export_run; replace them with real values, or export the full manifest with the rotari CLI")
	}
	format := input.Format
	if format == "" {
		format = "yaml"
	}
	manifest, err := workflow.Decode(strings.NewReader(input.Manifest), format)
	if err != nil {
		return workflowstate.Plan{}, err
	}
	runner := tools.runner()
	plan, err := workflowstate.Import{
		Store: runner.Store, BaseDir: baseDir, Project: paths.ProjectName, Manifest: manifest,
		Overwrite: input.Overwrite, Guard: guard, NewJobID: tools.options.NewJobID,
		Validate: func(queue model.Queue) error { return runner.ValidateQueue(queue, "", nil, nil) },
		Register: basedirregistry.Open(tools.masterDir).Register,
	}.Apply()
	return plan, err
}

// runPlanRequest returns the plan request of a run as `rotari run`, or with
// retry `rotari retry`, would make with its default options.
func runPlanRequest(retry bool) projectrun.PlanRequest {
	request := projectrun.PlanRequest{PartialArray: true, MatchBy: model.MatchByIDAndFingerprint}
	if retry {
		request.Selection = model.ResultSelection(true, true, false)
	}
	return request
}

// configureRunSource selects queue or saved-run snapshot construction. The
// supervisor resolves CopyIfEmpty while holding the state lock.
func configureRunSource(paths state.ProjectPaths, request *projectrun.PlanRequest, requestedRunID string) error {
	if requestedRunID != "" {
		resolved, err := resolve.RunID(paths, requestedRunID)
		if err != nil {
			return err
		}
		requestedRunID = resolved
	}
	sourceRunID, policy, err := projectrun.RunSource(paths, request.Selection, requestedRunID)
	if err != nil {
		return err
	}
	request.SourceRunID = sourceRunID
	request.SourcePolicy = policy
	return nil
}

// previewRun plans a run as rotari_start_run would start it.
func (tools writeTools) previewRun(input RunInput) (RunPreviewOutput, error) {
	_, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return RunPreviewOutput{}, err
	}
	request := runPlanRequest(input.Retry)
	if err := configureRunSource(paths, &request, input.RunID); err != nil {
		return RunPreviewOutput{}, err
	}
	planned, revision, err := tools.runner().PreviewRun(paths, nil, request, "")
	if err != nil {
		return RunPreviewOutput{}, err
	}
	output := RunPreviewOutput{
		Project: paths.ProjectName, Execute: []PlannedJob{}, Carried: len(planned.Plan.CarriedResults),
		SourceRun: planned.SourceRunID, OmittedSourceJobs: planned.OmittedSourceJobs, Revision: revision,
	}
	for _, job := range model.QueueToJobs(planned.Queue.Commands) {
		output.Jobs++
		if planned.Plan.Execute[job.ID] {
			output.Execute = append(output.Execute, PlannedJob{ID: job.ID, Name: job.Name, DependsOnRerun: planned.Plan.RerunDependencies[job.ID]})
		}
	}
	return output, nil
}

// startRun starts a run asynchronously, as `rotari run --async`, or with
// retry `rotari retry --async`, does, only at input.IfRevision.
func (tools writeTools) startRun(input StartRunInput) (StartRunOutput, error) {
	if tools.options.StartRun == nil {
		return StartRunOutput{}, errors.New("this MCP server cannot start runs")
	}
	if input.IfRevision == "" {
		return StartRunOutput{}, errors.New("if_revision is required; take it from rotari_preview_run")
	}
	_, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return StartRunOutput{}, err
	}
	request := runPlanRequest(input.Retry)
	if err := configureRunSource(paths, &request, input.RunID); err != nil {
		return StartRunOutput{}, err
	}
	planned, _, err := tools.runner().PreviewRun(paths, nil, request, input.IfRevision)
	if err != nil {
		return StartRunOutput{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return StartRunOutput{}, fmt.Errorf("failed to determine working directory: %w", err)
	}
	response, err := tools.options.StartRun(paths, server.Request{
		Op: server.OpRun, QueueName: paths.ProjectName, LocalConcurrency: 8, BatchMaxActive: 8, Async: true,
		RunName: input.RunName, EnvMode: model.EnvModeAll, CWD: cwd,
		Selection: request.Selection, SourceRunID: request.SourceRunID, PartialArray: request.PartialArray, MatchBy: request.MatchBy,
		SourcePolicy: string(request.SourcePolicy),
		IfRevision:   input.IfRevision,
	})
	if err != nil {
		return StartRunOutput{}, err
	}
	if !response.OK {
		return StartRunOutput{}, errors.New(response.Message)
	}
	return StartRunOutput{
		Project: paths.ProjectName, RunID: response.RunID,
		SourceRun: planned.SourceRunID, OmittedSourceJobs: planned.OmittedSourceJobs,
	}, nil
}
