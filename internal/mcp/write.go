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
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
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
	Retry      bool   `json:"retry,omitempty" jsonschema:"run only the failed and unfinished jobs and carry the other results, as rotari retry does: those of the queue, such as one just imported, or of the last run when the queue is empty"`
}

type StartRunInput struct {
	RunInput
	IfRevision string `json:"if_revision" jsonschema:"revision from rotari_preview_run; the run starts only if the project is still at it"`
	RunName    string `json:"run_name,omitempty" jsonschema:"label for the run"`
}

type PlannedJob struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type RunPreviewOutput struct {
	Project  string       `json:"project"`
	Execute  []PlannedJob `json:"execute" jsonschema:"jobs the run would execute"`
	Jobs     int          `json:"jobs" jsonschema:"jobs in the queue the run would start from"`
	Carried  int          `json:"carried" jsonschema:"results the run would carry forward instead of executing"`
	Revision string       `json:"revision" jsonschema:"revision to pass to rotari_start_run"`
}

type StartRunOutput struct {
	Project string `json:"project"`
	RunID   string `json:"run_id"`
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
	return projectrun.Runner{Store: store, Executors: executor.NewRegistry(store, func(string, ...any) {})}
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

// runQueue returns the queue a run would start from and the request's
// reference run: the project's queue, or the queue a copy from the last run
// leaves when projectrun.RunSource says to copy first. The copy runs under
// guard, so a dry run writes nothing.
func (tools writeTools) runQueue(paths state.ProjectPaths, request *projectrun.PlanRequest, guard project.Guard) (*model.Queue, error) {
	sourceRunID, copyFirst, err := projectrun.RunSource(paths, request.Selection, "")
	if err != nil {
		return nil, err
	}
	request.SourceRunID = sourceRunID
	if !copyFirst {
		return nil, nil
	}
	var outcome project.Outcome
	report := guard.Report
	guard.Report = func(result project.Outcome) {
		outcome = result
		if report != nil {
			report(result)
		}
	}
	runner := tools.runner()
	editor := queueops.Editor{Store: runner.Store, Executors: runner.Executors, NewJobID: tools.options.NewJobID, Guard: guard}
	if _, err := editor.Copy(paths.BaseDir, paths.ProjectName, sourceRunID, queueedit.CopyRequest{Selection: "all", Overwrite: true}); err != nil {
		return nil, err
	}
	return outcome.Queue, nil
}

// previewRun plans a run as rotari_start_run would start it.
func (tools writeTools) previewRun(input RunInput) (RunPreviewOutput, error) {
	_, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return RunPreviewOutput{}, err
	}
	request := runPlanRequest(input.Retry)
	queue, err := tools.runQueue(paths, &request, project.Guard{DryRun: true})
	if err != nil {
		return RunPreviewOutput{}, err
	}
	planned, revision, err := tools.runner().PreviewRun(paths, queue, request, "")
	if err != nil {
		return RunPreviewOutput{}, err
	}
	output := RunPreviewOutput{Project: paths.ProjectName, Execute: []PlannedJob{}, Carried: len(planned.Plan.CarriedResults), Revision: revision}
	for _, job := range model.QueueToJobs(planned.Queue.Commands) {
		output.Jobs++
		if planned.Plan.Execute[job.ID] {
			output.Execute = append(output.Execute, PlannedJob{ID: job.ID, Name: job.Name})
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
	revision := input.IfRevision
	guard := project.Guard{IfRevision: input.IfRevision, Report: func(outcome project.Outcome) { revision = outcome.NewRevision }}
	if _, err := tools.runQueue(paths, &request, guard); err != nil {
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
		IfRevision: revision,
	})
	if err != nil {
		return StartRunOutput{}, err
	}
	if !response.OK {
		return StartRunOutput{}, errors.New(response.Message)
	}
	return StartRunOutput{Project: paths.ProjectName, RunID: response.RunID}, nil
}
