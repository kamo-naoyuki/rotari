package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	// Revision is the project's revision: for --dry-run the one to pass to
	// --if-revision, and after an import the one it produced.
	Revision string `json:"revision"`
}

type importPlanJob struct {
	ID      string            `json:"id"`
	Name    string            `json:"name,omitempty"`
	Status  string            `json:"status"`
	Command []string          `json:"command"`
	Source  *importPlanSource `json:"source,omitempty"`
	Tasks   []importPlanTask  `json:"tasks,omitempty"`
}

// importPlanTask reports the status of one array task that has source
// provenance.
type importPlanTask struct {
	ID     string            `json:"id"`
	Status string            `json:"status"`
	Source *importPlanSource `json:"source,omitempty"`
}

// importPlanSource names the decoded source attempt so a reviewer can check
// which earlier result a queued job refers to.
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
	jsonOutput := cliBool(fs, "json", false)
	guard := cliGuardFlags(fs)
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
	var data []byte
	var err error
	if manifestPath == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(manifestPath)
	}
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
	outcome, err := writeImportedQueue(baseDir, resolvedProject, queue, *overwrite, guard.guard())
	if err != nil {
		printError(err)
		return 1
	}
	plan.Revision = outcome.Revision
	if outcome.Applied {
		plan.Revision = outcome.NewRevision
		if err := registerBasedir(baseDir); err != nil {
			printErrorf("failed to register state directory: %v", err)
			return 1
		}
	}
	if err := writeImportPlan(plan, *jsonOutput); err != nil {
		printErrorf("failed to encode import plan: %v", err)
		return 1
	}
	return 0
}

func writeImportPlan(plan importPlan, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	for _, job := range plan.Jobs {
		fields := append([]string{job.Status, "job_id=" + job.ID}, optionalField("job_name", job.Name)...)
		fields = append(fields, importSourceFields(job.Source)...)
		printImportPlanLine(job.Status, append(fields, importCommandField(job.Command)))
		for _, task := range job.Tasks {
			fields := append([]string{" ", task.Status, "task_id=" + task.ID}, importSourceFields(task.Source)...)
			printImportPlanLine(task.Status, fields)
		}
	}
	for _, removed := range plan.Removed {
		fields := append([]string{"remove", "job_id=" + removed.JobID}, optionalField("job_name", removed.Name)...)
		fields = append(fields, "source_run_id="+removed.RunID)
		printImportPlanLine("remove", append(fields, importCommandField(removed.Command)))
	}
	fmt.Printf("revision=%s\n", plan.Revision)
	return nil
}

// printImportPlanLine colors a plan line by the job's status. Successful
// results are cyan because they are informational; marked statuses and
// removals are yellow because they need attention; other jobs are green like
// other queue changes such as add.
func printImportPlanLine(status string, fields []string) {
	labelColor := green
	switch {
	case status == model.StatusSuccess:
		labelColor = cyan
	case status == "remove" || strings.HasSuffix(status, ")"):
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
	if path == "-" {
		return "json", nil
	}
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
	plan := importPlan{Version: 2, Project: project, Jobs: make([]importPlanJob, 0, len(queue.Commands)), Removed: removed}
	for _, command := range queue.Commands {
		job := importPlanJob{ID: command.ID, Name: command.Name, Command: command.Command}
		if command.Array == nil || len(command.TaskOrigins) == 0 {
			job.Status = importPlanStatus(command.Origin, command.MarkedStatus)
			job.Source = newImportPlanSource(command.Origin)
		} else {
			job.Tasks = importPlanTasks(command)
			job.Status = job.Tasks[0].Status
			for _, task := range job.Tasks[1:] {
				if task.Status != job.Status {
					job.Status = "mixed"
				}
			}
		}
		plan.Jobs = append(plan.Jobs, job)
	}
	return plan
}

// importPlanStatus is the status a job has in the imported queue, as `show`
// lists it: a job without a source result is unfinished.
func importPlanStatus(origin *model.JobOrigin, marked string) string {
	if text := model.QueuedStatusText(origin, marked); text != "-" {
		return text
	}
	return model.StatusUnfinished
}

func importPlanTasks(command model.QueuedCommand) []importPlanTask {
	tasks := make([]importPlanTask, 0, len(command.TaskOrigins))
	for _, task := range model.ArrayTaskIDs(command.Array) {
		taskID := fmt.Sprintf("%s-%d", command.ID, task)
		origin := command.TaskOrigins[taskID]
		status := importPlanStatus(origin, command.MarkedStatusOf(taskID))
		tasks = append(tasks, importPlanTask{ID: taskID, Status: status, Source: newImportPlanSource(origin)})
	}
	return tasks
}

func newImportPlanSource(origin *model.JobOrigin) *importPlanSource {
	if origin == nil {
		return nil
	}
	return &importPlanSource{RunID: origin.RunID, JobID: origin.JobID, AttemptID: origin.AttemptID, Status: origin.Status}
}

// writeImportedQueue replaces the project's queue with queue under guard. A
// dry run of a project that does not exist yet creates nothing and reports
// the revision a new project has.
func writeImportedQueue(baseDir, projectName string, queue model.Queue, overwrite bool, guard project.Guard) (project.Outcome, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return project.Outcome{}, err
	}
	var outcome project.Outcome
	report := guard.Report
	guard.Report = func(result project.Outcome) {
		outcome = result
		if report != nil {
			report(result)
		}
	}
	if _, err := os.Stat(paths.ProjectDir); errors.Is(err, os.ErrNotExist) && guard.DryRun {
		revision, err := project.CheckRevision(paths, guard)
		if err != nil {
			return project.Outcome{}, err
		}
		return project.Outcome{Revision: revision, Queue: &queue}, nil
	}
	if !guard.DryRun {
		if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
			return project.Outcome{}, err
		}
	}
	err = project.EditQueueGuarded(paths, "import", guard, func(existing *model.Queue) error {
		if len(existing.Commands) > 0 && !overwrite {
			return fmt.Errorf("project %q has queued jobs; use --overwrite", projectName)
		}
		*existing = queue
		return nil
	})
	return outcome, err
}
