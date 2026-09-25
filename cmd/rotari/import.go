package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

type importPlan struct {
	Version int             `json:"version"`
	Project string          `json:"project"`
	Jobs    []importPlanJob `json:"jobs"`
}

type importPlanJob struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Action string `json:"action"`
}

func cmdImport(args []string) int {
	manifestPath := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		manifestPath = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	overwrite := cliBool(fs, "overwrite", false)
	dryRun := cliBool(fs, "dry-run", false)
	jsonOutput := cliBool(fs, "json", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if manifestPath == "" && len(fs.Args()) == 1 {
		manifestPath = fs.Args()[0]
	} else if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("import"))
		return 1
	}
	if manifestPath == "" {
		printError("usage: " + cliUsage("import"))
		return 1
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		printErrorf("failed to read workflow manifest: %v", err)
		return 1
	}
	format, err := workflowFormatFromPath(manifestPath)
	if err != nil {
		printError(err)
		return 1
	}
	manifest, err := workflow.Decode(strings.NewReader(string(data)), format)
	if err != nil {
		printError(err)
		return 1
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printError(err)
		return 1
	}
	resolvedProject, err := resolveProjectName(baseDir, *projectName)
	if err != nil {
		printError(err)
		return 1
	}
	if manifest.Source != nil && manifest.Source.Project != resolvedProject {
		printErrorf("workflow source project %q does not match destination project %q", manifest.Source.Project, resolvedProject)
		return 1
	}
	queue, err := workflow.Compile(manifest, makeJobID)
	if err != nil {
		printError(err)
		return 1
	}
	queue, err = reconcileWorkflowManifest(baseDir, manifest, queue)
	if err != nil {
		printError(err)
		return 1
	}
	if err := validateQueueForRun(queue, "", nil, nil); err != nil {
		printErrorf("invalid workflow queue: %v", err)
		return 1
	}
	plan := newImportPlan(resolvedProject, queue)
	if *dryRun {
		if err := validateImportDestination(baseDir, resolvedProject, *overwrite); err != nil {
			printError(err)
			return 1
		}
	} else {
		if err := writeImportedQueue(baseDir, resolvedProject, queue, *overwrite); err != nil {
			printError(err)
			return 1
		}
	}
	if err := writeImportPlan(plan, *jsonOutput); err != nil {
		printErrorf("failed to encode import plan: %v", err)
		return 1
	}
	return 0
}

func validateImportDestination(baseDir, projectName string, overwrite bool) error {
	paths, err := resolvePaths(baseDir, projectName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(paths.ProjectDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	release, err := acquireStateReadLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue for reading: %w", err)
	}
	defer release()
	inspection, err := inspectConsistentProjectState(paths, false)
	if err != nil {
		return fmt.Errorf("failed to check project state: %w", err)
	}
	if inspection.State == projectRunning {
		return fmt.Errorf("project %q is running; import is not allowed", projectName)
	}
	if inspection.State == projectInterrupted {
		return fmt.Errorf("project %q has interrupted run %q; import is not allowed", projectName, inspection.RunID)
	}
	existing, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return err
	}
	if len(existing.Commands) > 0 && !overwrite {
		return fmt.Errorf("project %q has queued jobs; use --overwrite", projectName)
	}
	return nil
}

func writeImportPlan(plan importPlan, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	for _, job := range plan.Jobs {
		if job.Name == "" {
			fmt.Printf("%s job_id=%s\n", job.Action, job.ID)
		} else {
			fmt.Printf("%s job_id=%s job_name=%s\n", job.Action, job.ID, job.Name)
		}
	}
	return nil
}

func workflowFormatFromPath(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	case ".json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported workflow manifest extension %q", filepath.Ext(path))
	}
}

func newImportPlan(project string, queue Queue) importPlan {
	plan := importPlan{Version: 1, Project: project, Jobs: make([]importPlanJob, 0, len(queue.Commands))}
	for _, command := range queue.Commands {
		action := "execute"
		if command.Accepted || len(command.TaskAccepted) > 0 {
			action = "accept"
		} else if command.Origin != nil || len(command.TaskOrigins) > 0 {
			action = "reuse"
		}
		if command.Force || len(command.TaskForce) > 0 {
			action = "execute"
		}
		plan.Jobs = append(plan.Jobs, importPlanJob{ID: command.ID, Name: command.Name, Action: action})
	}
	return plan
}

func writeImportedQueue(baseDir, projectName string, queue Queue, overwrite bool) error {
	paths, err := resolvePaths(baseDir, projectName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.ProjectDir, stateDirMode()); err != nil {
		return err
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "import"); err != nil {
		return err
	}
	existing, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return err
	}
	if len(existing.Commands) > 0 && !overwrite {
		return fmt.Errorf("project %q has queued jobs; use --overwrite", projectName)
	}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		return fmt.Errorf("failed to write queue: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return err
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	return state.WriteJSON(paths.MetaFile, meta)
}
