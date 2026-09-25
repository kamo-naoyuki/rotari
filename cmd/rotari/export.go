package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

const workflowTemplateYAML = `version: 1

jobs:
  - name: example
    command: [echo, hello]

    # stage: prepare
    # depends_on: [other-job, prepare]
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
	var runIDs stringSliceFlag
	cliValue(fs, &runIDs, "run-id")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if !validWorkflowFormat(*format) || (*template && (len(fs.Args()) != 0 || cliOptionSet(fs, "run-id") || cliOptionSet(fs, "basedir") || cliOptionSet(fs, "project-name"))) {
		printError("usage: " + cliUsage("export"))
		return 1
	}
	selectedProject, selectedRunIDs, err := splitProjectOrRunSelectors(fs.Args())
	if err != nil {
		printError(err)
		return 1
	}
	if selectedProject != "" {
		if cliOptionSet(fs, "project-name") {
			printError("usage: " + cliUsage("export"))
			return 1
		}
		*projectName = selectedProject
	}
	runIDs = append(runIDs, selectedRunIDs...)
	if *template {
		data, err := workflowTemplate(*format)
		if err != nil {
			printError(err)
			return 1
		}
		_, err = os.Stdout.Write(data)
		if err != nil {
			printErrorf("failed to write workflow manifest: %v", err)
			return 1
		}
		return 0
	}
	manifest, err := exportWorkflow(*basedir, *projectName, runIDs)
	if err != nil {
		printError(err)
		return 1
	}
	data, err := workflow.Encode(manifest, *format)
	if err != nil {
		printErrorf("failed to encode workflow manifest: %v", err)
		return 1
	}
	if _, err := os.Stdout.Write(data); err != nil {
		printErrorf("failed to write workflow manifest: %v", err)
		return 1
	}
	return 0
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

func exportWorkflow(baseDir, projectName string, requestedRunIDs []string) (workflow.Manifest, error) {
	if len(requestedRunIDs) == 0 {
		return exportCurrentQueue(baseDir, projectName)
	}
	runs, selectedProject, err := loadExportRuns(baseDir, projectName, requestedRunIDs)
	if err != nil {
		return workflow.Manifest{}, err
	}
	return workflow.MergeRuns(selectedProject, runs)
}

func exportCurrentQueue(baseDir, projectName string) (workflow.Manifest, error) {
	resolvedBaseDir, _, err := resolveBaseDir(baseDir)
	if err != nil {
		return workflow.Manifest{}, err
	}
	resolvedProject, err := resolveProjectName(resolvedBaseDir, projectName)
	if err != nil {
		return workflow.Manifest{}, err
	}
	paths, err := resolvePaths(resolvedBaseDir, resolvedProject)
	if err != nil {
		return workflow.Manifest{}, err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return workflow.Manifest{}, fmt.Errorf("failed to load queue: %w", err)
	}
	queue = workflow.FlattenQueueDefaults(queue)
	if err := validateQueueForRun(queue, "", nil, nil); err != nil {
		return workflow.Manifest{}, fmt.Errorf("invalid queue: %w", err)
	}
	return workflow.FromQueue(queue)
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
		resolvedBaseDir, resolvedProject, err := resolveExistingRunTarget(baseDir, projectName, requested)
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
	paths, err := resolvePaths(baseDir, projectName)
	if err != nil {
		return workflow.SourceRun{}, err
	}
	runID, err := selectRunID(paths, requested)
	if err != nil {
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
	if err := validateQueueForRun(queue, "", nil, nil); err != nil {
		return workflow.SourceRun{}, fmt.Errorf("invalid run %s commands: %w", runID, err)
	}
	return workflowSourceRun(runID, runDir, queue, summary), nil
}
