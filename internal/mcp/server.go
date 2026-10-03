// Package mcp exposes rotari state through the Model Context Protocol.
package mcp

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/report"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetJobInfoInput struct {
	RunID string `json:"run_id" jsonschema:"exact run ID containing the job"`
	JobID string `json:"job_id" jsonschema:"exact job ID to inspect"`
}

type GetJobInfoOutput struct {
	Project string `json:"project"`
	RunID   string `json:"run_id"`
	JobID   string `json:"job_id"`
	Report  string `json:"report"`
}

// NewServer creates an MCP server for the runs and basedirs registered under
// masterDir. options supplies what its tools that change state need.
func NewServer(masterDir string, options Options) *mcpsdk.Server {
	writes := writeTools{masterDir: masterDir, options: options}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "rotari",
		Version: "0.1.0",
	}, &mcpsdk.ServerOptions{
		Instructions: "Inspect and run rotari projects. Start with rotari_list_projects to find recent runs and their results, " +
			"then rotari_run_summary for a run's failures grouped by cause (rotari_wait_run waits for a running one), rotari_get_job_info for one job's evidence, " +
			"and rotari_compare_runs to see what a later run fixed. Runs are named by run ID; the server finds their state directory " +
			"and project itself and returns no absolute paths. Paths and hostnames in logs and evidence are redacted where detected. " +
			"To change a project, preview first (rotari_preview_import, rotari_preview_run), show the user what will happen, " +
			"then apply with the preview's revision (rotari_import, rotari_start_run); a write fails if the project changed since. " +
			"To stop or pause a running run, list its jobs with rotari_preview_job_control, then call rotari_cancel, rotari_suspend, or rotari_resume with its run_id. " +
			"To clear a queue or recover an interrupted run, use rotari_preview_reset and rotari_reset.",
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_list_projects",
		Description: "List the projects of every registered rotari state directory with their state, queue size, run count, and last run's ID, status, and failed job count.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ ListProjectsInput) (ListProjectsOutput, error) {
		output, err := listProjects(masterDir)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_run_summary",
		Description: "Summarize one run: job counts, and failed and blocked jobs grouped by cause, each with up to 10 of its jobs (all_jobs lists all), exit codes, an example evidence line, the attempt to inspect, and a suggested fix.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input RunSummaryInput) (RunSummaryOutput, error) {
		output, err := runSummary(masterDir, input)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_wait_run",
		Description: "Wait up to timeout_seconds for a run to stop running, or with until_failure for a job to fail with no retry left, as rotari wait does, then return the run's summary as rotari_run_summary does and the reason it returned. Use it instead of polling rotari_run_summary.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, input WaitRunInput) (WaitRunOutput, error) {
		return waitRun(ctx, masterDir, input)
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_get_job_info",
		Description: "Return a redacted AI-oriented report for one rotari job, including its status, result, diagnosis, and the log lines around the diagnosis evidence.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input GetJobInfoInput) (GetJobInfoOutput, error) {
		output, err := getJobInfo(masterDir, input)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_check_project",
		Description: "Report whether a project's queued run can start: its state (ready, empty, running, locked, or interrupted), queued job count, and lock, after validating the queue's jobs, dependencies, and executors.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input CheckProjectInput) (CheckProjectOutput, error) {
		output, err := checkProject(masterDir, input)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_compare_runs",
		Description: "Compare two runs of one project: which jobs were fixed, still fail, or newly fail, each run's failure cause and whether it changed, and which job definitions changed. Jobs that did not change are only counted, and at most 20 changed jobs are listed, unless all_jobs is set.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input CompareRunsInput) (CompareRunsOutput, error) {
		output, err := compareRuns(masterDir, input)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_export_run",
		Description: "Export a finished run as a workflow manifest, as rotari export RUN_ID does, for reading: environment values, executor options, and paths are redacted, so it cannot be imported until they are replaced with real values.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input ExportRunInput) (ExportRunOutput, error) {
		output, err := exportRun(masterDir, input)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_preview_import",
		Description: "Preview importing a workflow manifest into a project's queue, as rotari import --dry-run does: the jobs it would queue with their statuses (an array's tasks counted by status), status counts, the source jobs it would drop, and the project revision; detail adds each command and source attempt. Changes nothing.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input ImportInput) (ImportOutput, error) {
		output, err := writes.importManifest(input, project.Guard{DryRun: true})
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_import",
		Description: "Import a workflow manifest into a project's queue, as rotari import does, only if the project is still at the revision rotari_preview_import returned. Returns the plan's summary, as the preview does, and the new revision.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPointer(true)},
	}, func(_ context.Context, input ApplyImportInput) (ImportOutput, error) {
		if input.IfRevision == "" {
			return ImportOutput{}, errors.New("if_revision is required; take it from rotari_preview_import")
		}
		output, err := writes.importManifest(input.ImportInput, project.Guard{IfRevision: input.IfRevision})
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_preview_run",
		Description: "Preview a run of a project, as rotari run --dry-run (or with retry, rotari retry --dry-run) does: the jobs it would execute, the results it would carry, and the project revision. Changes nothing.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input RunInput) (RunPreviewOutput, error) {
		output, err := writes.previewRun(input)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_start_run",
		Description: "Start a run of a project in the background, as rotari run --async (or with retry, rotari retry --async) does, only if the project is still at the revision rotari_preview_run returned. Returns the run ID; follow it with rotari_wait_run.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPointer(false)},
	}, func(_ context.Context, input StartRunInput) (StartRunOutput, error) {
		output, err := writes.startRun(input)
		return output, err
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_preview_reset",
		Description: "Preview resetting a project, as rotari reset --dry-run does: how many queued jobs it would remove, and any interrupted run it would recover, with what that run's jobs last reported, and the project revision. Run history is kept. Changes nothing.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input ResetInput) (ResetOutput, error) {
		return writes.reset(input, true, project.Guard{DryRun: true})
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_reset",
		Description: "Reset a project, as rotari reset does, only if it is still at the revision rotari_preview_reset returned: remove its queued jobs, keeping run history, and with recover_interrupted recover its interrupted run. A running project is refused.",
		Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPointer(true)},
	}, func(_ context.Context, input ApplyResetInput) (ResetOutput, error) {
		return writes.applyReset(input)
	})
	addTool(server, masterDir, &mcpsdk.Tool{
		Name:        "rotari_preview_job_control",
		Description: "List the unfinished jobs of a running run that cancel (running and pending jobs), suspend, or resume (running jobs) would act on, optionally only those with given names. Changes nothing.",
		Annotations: &mcpsdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, input JobControlInput) (JobControlPreview, error) {
		return writes.previewJobControl(input)
	})
	for _, control := range []struct {
		operation, description string
		destructive            bool
	}{
		{"cancel", "Cancel jobs of a running run, as rotari cancel --run-id does: the job_ids from rotari_preview_job_control, or without them the whole run, which then starts no more jobs. Cancelled jobs do not resume; rerun them with rotari_start_run and retry.", true},
		{"suspend", "Suspend running jobs of a running run, as rotari suspend --run-id does: the job_ids from rotari_preview_job_control, or without them every running job. rotari_resume continues them.", false},
		{"resume", "Resume suspended jobs of a running run, as rotari resume --run-id does: the job_ids from rotari_preview_job_control, or without them every running job.", false},
	} {
		operation := control.operation
		addTool(server, masterDir, &mcpsdk.Tool{
			Name:        "rotari_" + operation,
			Description: control.description,
			Annotations: &mcpsdk.ToolAnnotations{DestructiveHint: boolPointer(control.destructive)},
		}, func(_ context.Context, input ControlJobsInput) (ControlJobsOutput, error) {
			return writes.controlJobs(input, operation)
		})
	}
	return server
}

// addTool adds a tool whose errors name no state directory; see
// hideStateDirs. Every tool is added through it.
func addTool[In, Out any](server *mcpsdk.Server, masterDir string, tool *mcpsdk.Tool, handle func(context.Context, In) (Out, error)) {
	mcpsdk.AddTool(server, tool, func(ctx context.Context, _ *mcpsdk.CallToolRequest, input In) (*mcpsdk.CallToolResult, Out, error) {
		output, err := handle(ctx, input)
		return nil, output, hideStateDirs(masterDir, err)
	})
}

// hideStateDirs replaces in err's message each directory registered under
// masterDir with BASEDIR, and masterDir with MASTERDIR.
func hideStateDirs(masterDir string, err error) error {
	if err == nil {
		return nil
	}
	baseDirs, _, _ := basedirregistry.Discover(masterDir)
	// A longer directory is replaced first, so one nested in another is not
	// left partly named.
	sort.Slice(baseDirs, func(i, j int) bool { return len(baseDirs[i]) > len(baseDirs[j]) })
	message := err.Error()
	for _, baseDir := range baseDirs {
		message = hideBaseDir(message, baseDir)
	}
	return errors.New(strings.ReplaceAll(message, masterDir, "MASTERDIR"))
}

// hideBaseDir replaces baseDir in text with BASEDIR.
func hideBaseDir(text, baseDir string) string {
	if baseDir == "" {
		return text
	}
	return strings.ReplaceAll(text, baseDir, "BASEDIR")
}

func boolPointer(value bool) *bool {
	return &value
}

// getJobInfo builds the redacted report for one explicitly selected job of a
// run registered under masterDir.
func getJobInfo(masterDir string, input GetJobInfoInput) (GetJobInfoOutput, error) {
	location, err := resolve.RegisteredRun(masterDir, input.RunID)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	paths, err := state.ResolveProjectPaths(location.BaseDir, location.ProjectName)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	jobReport, err := report.Build(store, paths, location.RunID, input.JobID, false, "", true)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	return GetJobInfoOutput{
		Project: location.ProjectName,
		RunID:   location.RunID,
		JobID:   input.JobID,
		Report:  jobReport,
	}, nil
}
