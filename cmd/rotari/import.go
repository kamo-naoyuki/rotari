package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

type importPlan struct {
	Version int                  `json:"version"`
	Project string               `json:"project"`
	Jobs    []importPlanJob      `json:"jobs"`
	Removed []workflowRemovedJob `json:"removed"`
}

type importPlanJob struct {
	ID     string            `json:"id"`
	Name   string            `json:"name,omitempty"`
	Action string            `json:"action"`
	Source *importPlanSource `json:"source,omitempty"`
	Tasks  []importPlanTask  `json:"tasks,omitempty"`
}

// importPlanTask reports the disposition of one array task that has source
// provenance.
type importPlanTask struct {
	ID     string            `json:"id"`
	Action string            `json:"action"`
	Source *importPlanSource `json:"source,omitempty"`
}

// importPlanSource names the decoded source attempt so a reviewer can check
// which earlier result a reused, accepted, or re-executed job refers to.
type importPlanSource struct {
	RunID     string `json:"run_id"`
	JobID     string `json:"job_id"`
	AttemptID string `json:"attempt_id,omitempty"`
	Status    string `json:"status"`
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
	queue, removed, err := reconcileWorkflowManifest(baseDir, manifest, queue)
	if err != nil {
		printError(err)
		return 1
	}
	if err := validateQueueForRun(queue, "", nil, nil); err != nil {
		printErrorf("invalid workflow queue: %v", err)
		return 1
	}
	plan := newImportPlan(resolvedProject, queue, removed)
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
		fields := append([]string{job.Action, "job_id=" + job.ID}, optionalField("job_name", job.Name)...)
		fmt.Println(strings.Join(append(fields, importSourceFields(job.Source)...), " "))
		for _, task := range job.Tasks {
			fields := append([]string{" ", task.Action, "task_id=" + task.ID}, importSourceFields(task.Source)...)
			fmt.Println(strings.Join(fields, " "))
		}
	}
	for _, removed := range plan.Removed {
		fields := append([]string{"remove", "job_id=" + removed.JobID}, optionalField("job_name", removed.Name)...)
		fmt.Println(strings.Join(append(fields, "source_run_id="+removed.RunID), " "))
	}
	return nil
}

func importSourceFields(source *importPlanSource) []string {
	if source == nil {
		return nil
	}
	fields := []string{"source_run_id=" + source.RunID, "source_job_id=" + source.JobID}
	fields = append(fields, optionalField("source_attempt_id", source.AttemptID)...)
	return append(fields, "source_status="+source.Status)
}

func optionalField(key, value string) []string {
	if value == "" {
		return nil
	}
	return []string{key + "=" + value}
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

func newImportPlan(project string, queue Queue, removed []workflowRemovedJob) importPlan {
	if removed == nil {
		removed = []workflowRemovedJob{}
	}
	plan := importPlan{Version: 1, Project: project, Jobs: make([]importPlanJob, 0, len(queue.Commands)), Removed: removed}
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
		job := importPlanJob{ID: command.ID, Name: command.Name, Action: action}
		if command.Array == nil {
			job.Source = newImportPlanSource(command.Origin)
		} else if len(command.TaskOrigins) > 0 {
			job.Tasks = importPlanTasks(command)
		}
		plan.Jobs = append(plan.Jobs, job)
	}
	return plan
}

func importPlanTasks(command QueuedCommand) []importPlanTask {
	tasks := make([]importPlanTask, 0, len(command.TaskOrigins))
	for _, task := range model.ArrayTaskIDs(command.Array) {
		taskID := fmt.Sprintf("%s-%d", command.ID, task)
		origin := command.TaskOrigins[taskID]
		action := "execute"
		switch {
		case command.Force || command.TaskForce[taskID]:
		case command.TaskAccepted[taskID]:
			action = "accept"
		case origin != nil:
			action = "reuse"
		}
		tasks = append(tasks, importPlanTask{ID: taskID, Action: action, Source: newImportPlanSource(origin)})
	}
	return tasks
}

func newImportPlanSource(origin *JobOrigin) *importPlanSource {
	if origin == nil {
		return nil
	}
	return &importPlanSource{RunID: origin.RunID, JobID: origin.JobID, AttemptID: origin.AttemptID, Status: origin.Status}
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
