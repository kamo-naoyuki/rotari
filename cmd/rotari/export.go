package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

const workflowTemplateYAML = `version: 1

jobs:
  - name: example
    command: [echo, hello]

    # stage: prepare
    # depends_on: [other-job, prepare]
    # Start after these finish, whatever their result.
    # depends_on_finished: [sweep]
    # Stop the job this long after it starts; it then fails with exit code 124.
    # timeout: 2h
    # Retry this job up to N times when it fails, instead of run --retry.
    # retry: 2
    # Wait before retrying, multiplying the wait for each further retry.
    # retry_delay: 30s
    # retry_backoff: 2
    # retry_max_delay: 5m
    # executor: slurm
    # executor_options: ["--partition=gpu", "--gres=gpu:1"]
    # working_directory: ./work
    # environment: ["EPOCHS=20", "DATA_ROOT=./data"]

    # Same syntax as rotari add --array.
    # array: "1-10"
    # array: "1,3,4"

    # Cartesian expansion, with the same syntax as rotari add --matrix.
    # matrix: ["SEED=1,2,3", "MODEL=small,large"]
`

const workflowTemplateTOML = `version = 1

[[jobs]]
name = "example"
command = ["echo", "hello"]

# stage = "prepare"
# depends_on = ["other-job", "prepare"]
# Start after these finish, whatever their result.
# depends_on_finished = ["sweep"]
# Stop the job this long after it starts; it then fails with exit code 124.
# timeout = "2h"
# Retry this job up to N times when it fails, instead of run --retry.
# retry = 2
# Wait before retrying, multiplying the wait for each further retry.
# retry_delay = "30s"
# retry_backoff = 2
# retry_max_delay = "5m"
# executor = "slurm"
# executor_options = ["--partition=gpu", "--gres=gpu:1"]
# working_directory = "./work"
# environment = ["EPOCHS=20", "DATA_ROOT=./data"]
# array = "1-10"
# matrix = ["SEED=1,2,3", "MODEL=small,large"]
`

func cmdExport(args []string) int {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	format := cliString(fs, "format", "yaml")
	template := cliBool(fs, "template", false)
	outputPath := cliString(fs, "output", "")
	var runIDs stringSliceFlag
	cliValue(fs, &runIDs, "run-id")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	positional := fs.Args()
	if len(positional) > 2 {
		printError("usage: " + cliUsage("export"))
		return 1
	}
	if len(positional) == 2 {
		if cliOptionSet(fs, "output") {
			printError("usage: " + cliUsage("export"))
			return 1
		}
		*outputPath = positional[1]
		positional = positional[:1]
	}
	if !validWorkflowFormat(*format) || (*template && (len(positional) != 0 || cliOptionSet(fs, "run-id") || cliOptionSet(fs, "basedir") || cliOptionSet(fs, "project-name"))) {
		printError("usage: " + cliUsage("export"))
		return 1
	}
	selectedRunIDs := []string(nil)
	if len(positional) == 1 {
		// TARGET is a run when it looks like one, whatever the other options
		// say; otherwise it names the project and replaces --project-name.
		switch {
		case resolve.IsRunID(positional[0]) || positional[0] == "latest":
			selectedRunIDs = []string{positional[0]}
		case cliOptionSet(fs, "project-name"):
			printError("a project TARGET cannot be combined with --project-name; pass a run ID or drop the option")
			return 1
		default:
			*projectName = positional[0]
		}
	}
	runIDs = append(runIDs, selectedRunIDs...)
	if *template {
		data, err := workflowTemplate(*format)
		if err != nil {
			printError(err)
			return 1
		}
		if err := writeWorkflowManifest(data, *outputPath); err != nil {
			printError(err)
			return 1
		}
		return 0
	}
	manifest, notice, err := exportWorkflow(*basedir, *projectName, runIDs)
	if err != nil {
		printError(err)
		return 1
	}
	if notice != "" {
		fmt.Fprintln(os.Stderr, notice)
	}
	data, err := workflow.Encode(manifest, *format)
	if err != nil {
		printErrorf("failed to encode workflow manifest: %v", err)
		return 1
	}
	if err := writeWorkflowManifest(data, *outputPath); err != nil {
		printError(err)
		return 1
	}
	return 0
}

func writeWorkflowManifest(data []byte, outputPath string) error {
	if outputPath == "" {
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("failed to write workflow manifest: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write workflow manifest: %w", err)
	}
	return nil
}

func validWorkflowFormat(format string) bool {
	return format == "yaml" || format == "toml" || format == "json"
}

func workflowTemplate(format string) ([]byte, error) {
	switch format {
	case "yaml":
		return []byte(workflowTemplateYAML), nil
	case "toml":
		return []byte(workflowTemplateTOML), nil
	}
	return workflow.Encode(workflow.Manifest{Version: workflow.Version, Jobs: []workflow.Job{{Name: "example", Command: []string{"echo", "hello"}}}}, format)
}

// exportWorkflow exports the requested runs, or without any the project,
// and returns a notice naming the source when the project's queue was empty
// and its latest run was exported instead.
func exportWorkflow(baseDir, projectName string, requestedRunIDs []string) (workflow.Manifest, string, error) {
	if len(requestedRunIDs) == 0 {
		return exportProject(baseDir, projectName)
	}
	runs, selectedProject, err := loadExportRuns(baseDir, projectName, requestedRunIDs)
	if err != nil {
		return workflow.Manifest{}, "", err
	}
	manifest, err := workflow.MergeRuns(selectedProject, runs)
	return manifest, "", err
}

// exportProject picks a project's source as show does: a non-empty queue,
// else the latest run. An active or interrupted run is not settled, so it is
// refused rather than exported.
func exportProject(baseDir, projectName string) (workflow.Manifest, string, error) {
	resolvedBaseDir, _, err := state.ResolveBaseDir(baseDir)
	if err != nil {
		return workflow.Manifest{}, "", err
	}
	resolvedProject, err := state.ResolveProjectName(resolvedBaseDir, projectName)
	if err != nil {
		return workflow.Manifest{}, "", err
	}
	if err := resolve.RequireProject(resolvedBaseDir, resolvedProject); err != nil {
		return workflow.Manifest{}, "", err
	}
	paths, err := state.ResolveProjectPaths(resolvedBaseDir, resolvedProject)
	if err != nil {
		return workflow.Manifest{}, "", err
	}
	if err := requireSettledRun(paths, ""); err != nil {
		return workflow.Manifest{}, "", err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return workflow.Manifest{}, "", fmt.Errorf("failed to load queue: %w", err)
	}
	if len(queue.Commands) == 0 {
		runID, err := resolve.RunID(paths, "")
		if err != nil {
			return workflow.Manifest{}, "", fmt.Errorf("project %q has no queued jobs or runs to export", resolvedProject)
		}
		run, err := loadExportRun(resolvedBaseDir, resolvedProject, runID)
		if err != nil {
			return workflow.Manifest{}, "", err
		}
		manifest, err := workflow.MergeRuns(resolvedProject, []workflow.SourceRun{run})
		return manifest, fmt.Sprintf("exported run %s (project %s has no queued jobs)", runID, resolvedProject), err
	}
	queue = workflow.FlattenQueueDefaults(queue)
	if err := projectRunner().ValidateQueue(queue, "", nil, nil); err != nil {
		return workflow.Manifest{}, "", fmt.Errorf("invalid queue: %w", err)
	}
	manifest, err := workflow.FromQueue(queue)
	return manifest, "", err
}

// requireSettledRun refuses an active or interrupted run of the project:
// runID, or any when runID is empty. Their results are not final, so a
// manifest of them would re-execute jobs that may yet succeed.
func requireSettledRun(paths state.ProjectPaths, runID string) error {
	inspection, err := project.Inspect(paths, true)
	if err != nil {
		return fmt.Errorf("failed to check project state: %w", err)
	}
	if runID != "" && inspection.RunID != runID {
		return nil
	}
	switch inspection.State {
	case project.Running:
		return fmt.Errorf("run %s of project %s is still running; wait for it with 'rotari wait %s', then export", inspection.RunID, paths.ProjectName, paths.ProjectName)
	case project.Interrupted:
		return fmt.Errorf("run %s of project %s was interrupted; recover it with 'rotari unlock %s' first", inspection.RunID, paths.ProjectName, paths.ProjectName)
	}
	return nil
}

func loadExportRuns(baseDir, projectName string, requestedRunIDs []string) ([]workflow.SourceRun, string, error) {
	seen := make(map[string]bool, len(requestedRunIDs))
	var selectedBaseDir, selectedProject string
	runs := make([]workflow.SourceRun, 0, len(requestedRunIDs))
	for _, requested := range requestedRunIDs {
		if seen[requested] {
			return nil, "", fmt.Errorf("duplicate run ID %q", requested)
		}
		seen[requested] = true
		resolvedBaseDir, resolvedProject, err := resolve.ExistingRun(baseDir, projectName, requested)
		if err != nil {
			return nil, "", err
		}
		if selectedBaseDir == "" {
			selectedBaseDir, selectedProject = resolvedBaseDir, resolvedProject
		} else if selectedBaseDir != resolvedBaseDir || selectedProject != resolvedProject {
			return nil, "", errors.New("all exported runs must belong to the same project")
		}
		run, err := loadExportRun(resolvedBaseDir, resolvedProject, requested)
		if err != nil {
			return nil, "", err
		}
		runs = append(runs, run)
	}
	return runs, selectedProject, nil
}

func loadExportRun(baseDir, projectName, requested string) (workflow.SourceRun, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return workflow.SourceRun{}, err
	}
	runID, err := resolve.RunID(paths, requested)
	if err != nil {
		return workflow.SourceRun{}, err
	}
	if err := requireSettledRun(paths, runID); err != nil {
		return workflow.SourceRun{}, err
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load run %s commands: %w", runID, err)
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load run %s summary: %w", runID, err)
	}
	queue = workflow.FlattenQueueDefaults(queue)
	if err := projectRunner().ValidateQueue(queue, "", nil, nil); err != nil {
		return workflow.SourceRun{}, fmt.Errorf("invalid run %s commands: %w", runID, err)
	}
	return workflowSourceRun(runID, runDir, queue, summary), nil
}
