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
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

type importPlan struct {
	Version int                   `json:"version"`
	Project string                `json:"project"`
	Jobs    []importPlanJob       `json:"jobs"`
	Removed []workflow.RemovedJob `json:"removed"`
}

type importPlanJob struct {
	ID      string            `json:"id"`
	Name    string            `json:"name,omitempty"`
	Action  string            `json:"action"`
	Command []string          `json:"command"`
	Source  *importPlanSource `json:"source,omitempty"`
	Tasks   []importPlanTask  `json:"tasks,omitempty"`
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
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	positional := fs.Args()
	if manifestPath == "" && len(positional) > 0 {
		manifestPath = positional[0]
		positional = positional[1:]
	}
	if manifestPath == "" || len(positional) > 1 {
		printError("usage: " + cliUsage("import"))
		return 1
	}
	if len(positional) == 1 && cliOptionSet(fs, "project-name") {
		printError("usage: " + cliUsage("import"))
		return 1
	}
	if len(positional) == 1 {
		*projectName = positional[0]
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
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printError(err)
		return 1
	}
	resolvedProject, err := state.ResolveProjectName(baseDir, *projectName)
	if err != nil {
		printError(err)
		return 1
	}
	if !resolve.ProjectExists(baseDir, resolvedProject) {
		if err := model.ValidateReservedName("project", resolvedProject); err != nil {
			printError(err)
			return 1
		}
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
	if err := model.ValidateReservedNames(queue.Commands); err != nil {
		printError(err)
		return 1
	}
	if err := projectRunner().ValidateQueue(queue, "", nil, nil); err != nil {
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
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(paths.ProjectDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	release, err := state.AcquireStateReadLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue for reading: %w", err)
	}
	defer release()
	inspection, err := project.InspectConsistent(paths, false)
	if err != nil {
		return fmt.Errorf("failed to check project state: %w", err)
	}
	if inspection.State == project.Running {
		return fmt.Errorf("project %q is running; import is not allowed", projectName)
	}
	if inspection.State == project.Interrupted {
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
		fields = append(fields, importSourceFields(job.Source)...)
		printImportPlanLine(job.Action, append(fields, importCommandField(job.Command)))
		for _, task := range job.Tasks {
			fields := append([]string{" ", task.Action, "task_id=" + task.ID}, importSourceFields(task.Source)...)
			printImportPlanLine(task.Action, fields)
		}
	}
	for _, removed := range plan.Removed {
		fields := append([]string{"remove", "job_id=" + removed.JobID}, optionalField("job_name", removed.Name)...)
		fields = append(fields, "source_run_id="+removed.RunID)
		printImportPlanLine("remove", append(fields, importCommandField(removed.Command)))
	}
	return nil
}

// printImportPlanLine colors a plan line by its action. Queued work is green
// like other successful queue changes such as add; reused results are cyan
// because they are informational and do not execute; accepted failures and
// removals are yellow because they need attention.
func printImportPlanLine(action string, fields []string) {
	labelColor := green
	switch action {
	case "reuse":
		labelColor = cyan
	case "accept", "remove":
		labelColor = yellow
	}
	fmt.Println(colorKeyValueMessage(strings.Join(fields, " "), labelColor))
}

// importCommandField must be the last field: its value runs to the end of the
// line and may contain spaces.
func importCommandField(command []string) string {
	return "command=" + strings.Join(command, " ")
}

func importSourceFields(source *importPlanSource) []string {
	if source == nil {
		return nil
	}
	// The attempt ID encodes the source run and job IDs, so the human view
	// omits them; JSON output keeps every field.
	return append(optionalField("source_attempt_id", source.AttemptID), "source_status="+source.Status)
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

func newImportPlan(project string, queue model.Queue, removed []workflow.RemovedJob) importPlan {
	if removed == nil {
		removed = []workflow.RemovedJob{}
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
		job := importPlanJob{ID: command.ID, Name: command.Name, Action: action, Command: command.Command}
		if command.Array == nil {
			job.Source = newImportPlanSource(command.Origin)
		} else if len(command.TaskOrigins) > 0 {
			job.Tasks = importPlanTasks(command)
		}
		plan.Jobs = append(plan.Jobs, job)
	}
	return plan
}

func importPlanTasks(command model.QueuedCommand) []importPlanTask {
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

func newImportPlanSource(origin *model.JobOrigin) *importPlanSource {
	if origin == nil {
		return nil
	}
	return &importPlanSource{RunID: origin.RunID, JobID: origin.JobID, AttemptID: origin.AttemptID, Status: origin.Status}
}

func writeImportedQueue(baseDir, projectName string, queue model.Queue, overwrite bool) error {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
		return err
	}
	return project.EditQueue(paths, "import", func(existing *model.Queue) error {
		if len(existing.Commands) > 0 && !overwrite {
			return fmt.Errorf("project %q has queued jobs; use --overwrite", projectName)
		}
		*existing = queue
		return nil
	})
}
