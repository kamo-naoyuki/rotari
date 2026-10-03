package workflowstate

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

// Plan is what an import does to a project's queue: the jobs it queues and
// their source results, the source jobs it drops, and the project revision.
type Plan struct {
	Version int                   `json:"version"`
	Project string                `json:"project"`
	Jobs    []PlanJob             `json:"jobs"`
	Removed []workflow.RemovedJob `json:"removed"`
	// Revision is the project's revision: for a dry run the one to pass as
	// project.Guard.IfRevision, and after an import the one it produced.
	Revision string `json:"revision"`
}

// PlanJob is one queued job of a Plan.
type PlanJob struct {
	ID      string      `json:"id"`
	Name    string      `json:"name,omitempty"`
	Status  string      `json:"status"`
	Command []string    `json:"command"`
	Source  *PlanSource `json:"source,omitempty"`
	Tasks   []PlanTask  `json:"tasks,omitempty"`
}

// PlanTask reports the status of one array task that has source
// provenance.
type PlanTask struct {
	ID     string      `json:"id"`
	Status string      `json:"status"`
	Source *PlanSource `json:"source,omitempty"`
}

// PlanSource names the decoded source attempt so a reviewer can check which
// earlier result a queued job refers to.
type PlanSource struct {
	RunID     string `json:"run_id"`
	JobID     string `json:"job_id"`
	AttemptID string `json:"attempt_id,omitempty"`
	Status    string `json:"status"`
}

// PlanSummary is a Plan without commands and source attempts: each job's
// status, an array's tasks counted by status, and the statuses counted over
// the plan, where an array with task statuses counts each task instead of
// itself.
type PlanSummary struct {
	Project  string                `json:"project"`
	Counts   map[string]int        `json:"counts"`
	Jobs     []PlanJobSummary      `json:"jobs"`
	Removed  []workflow.RemovedJob `json:"removed"`
	Revision string                `json:"revision"`
}

// PlanJobSummary is one job of a PlanSummary.
type PlanJobSummary struct {
	ID     string         `json:"id"`
	Name   string         `json:"name,omitempty"`
	Status string         `json:"status"`
	Tasks  map[string]int `json:"tasks,omitempty"`
}

// Summary summarizes plan.
func (plan Plan) Summary() PlanSummary {
	summary := PlanSummary{Project: plan.Project, Counts: map[string]int{}, Jobs: make([]PlanJobSummary, 0, len(plan.Jobs)), Removed: plan.Removed, Revision: plan.Revision}
	for _, job := range plan.Jobs {
		brief := PlanJobSummary{ID: job.ID, Name: job.Name, Status: job.Status}
		if len(job.Tasks) == 0 {
			summary.Counts[job.Status]++
		} else {
			brief.Tasks = map[string]int{}
			for _, task := range job.Tasks {
				brief.Tasks[task.Status]++
				summary.Counts[task.Status]++
			}
		}
		summary.Jobs = append(summary.Jobs, brief)
	}
	return summary
}

// Import is one import of a manifest into a project.
type Import struct {
	Store    state.Store
	BaseDir  string
	Project  string
	Manifest workflow.Manifest
	// Overwrite allows replacing a queue that has jobs.
	Overwrite bool
	// Guard previews the import or applies it only at a given revision.
	Guard project.Guard
	// NewJobID returns a fresh job ID for the compiled queue.
	NewJobID func() string
	// Validate checks that the compiled queue can run, such as that its
	// executors are known.
	Validate func(model.Queue) error
	// Register, when set, records an imported project's basedir for
	// discovery once the import is written.
	Register func(baseDir string) error
}

// Apply compiles the manifest, reconciles it with its source runs, checks
// it, and writes it as the project's queue under the guard. It returns the
// plan, with the revision before a dry run or after a written import.
func (request Import) Apply() (Plan, error) {
	if !resolve.ProjectExists(request.BaseDir, request.Project) {
		if err := model.ValidateReservedName("project", request.Project); err != nil {
			return Plan{}, err
		}
	}
	manifest := request.Manifest
	if manifest.Source != nil && manifest.Source.Project != request.Project {
		return Plan{}, fmt.Errorf("workflow source project %q does not match destination project %q", manifest.Source.Project, request.Project)
	}
	queue, err := workflow.Compile(manifest, request.NewJobID)
	if err != nil {
		return Plan{}, err
	}
	queue, removed, err := Reconcile(request.Store, request.BaseDir, manifest, queue)
	if err != nil {
		return Plan{}, err
	}
	if err := model.ValidateReservedNames(queue.Commands); err != nil {
		return Plan{}, err
	}
	if err := request.Validate(queue); err != nil {
		return Plan{}, fmt.Errorf("invalid workflow queue: %w", err)
	}
	plan := NewPlan(request.Project, queue, removed)
	outcome, err := writeQueue(request.BaseDir, request.Project, queue, request.Overwrite, request.Guard)
	if err != nil {
		return Plan{}, err
	}
	plan.Revision = outcome.Revision
	if outcome.Applied {
		plan.Revision = outcome.NewRevision
		if request.Register != nil {
			if err := request.Register(request.BaseDir); err != nil {
				return Plan{}, fmt.Errorf("failed to register state directory: %w", err)
			}
		}
	}
	return plan, nil
}

// writeQueue replaces the project's queue with queue under guard, creating
// the project when needed; see project.CreateQueueGuarded.
func writeQueue(baseDir, projectName string, queue model.Queue, overwrite bool, guard project.Guard) (project.Outcome, error) {
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
	err = project.CreateQueueGuarded(paths, "import", guard, func(existing *model.Queue) error {
		if len(existing.Commands) > 0 && !overwrite {
			return fmt.Errorf("project %q has queued jobs; use --overwrite", projectName)
		}
		*existing = queue
		return nil
	})
	return outcome, err
}

// NewPlan describes queue, compiled for projectName, as an import plan.
func NewPlan(projectName string, queue model.Queue, removed []workflow.RemovedJob) Plan {
	if removed == nil {
		removed = []workflow.RemovedJob{}
	}
	plan := Plan{Version: 2, Project: projectName, Jobs: make([]PlanJob, 0, len(queue.Commands)), Removed: removed}
	for _, command := range queue.Commands {
		job := PlanJob{ID: command.ID, Name: command.Name, Command: command.Command}
		if command.Array == nil || len(command.TaskOrigins) == 0 {
			job.Status = planStatus(command.Origin, command.MarkedStatus)
			job.Source = newPlanSource(command.Origin)
		} else {
			job.Tasks = planTasks(command)
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

// planStatus is the status a job has in the imported queue, as `show` lists
// it: a job without a source result is unfinished.
func planStatus(origin *model.JobOrigin, marked string) string {
	if text := model.QueuedStatusText(origin, marked); text != "-" {
		return text
	}
	return model.StatusUnfinished
}

func planTasks(command model.QueuedCommand) []PlanTask {
	tasks := make([]PlanTask, 0, len(command.TaskOrigins))
	for _, task := range model.ArrayTaskIDs(command.Array) {
		taskID := fmt.Sprintf("%s-%d", command.ID, task)
		origin := command.TaskOrigins[taskID]
		status := planStatus(origin, command.MarkedStatusOf(taskID))
		tasks = append(tasks, PlanTask{ID: taskID, Status: status, Source: newPlanSource(origin)})
	}
	return tasks
}

func newPlanSource(origin *model.JobOrigin) *PlanSource {
	if origin == nil {
		return nil
	}
	return &PlanSource{RunID: origin.RunID, JobID: origin.JobID, AttemptID: origin.AttemptID, Status: origin.Status}
}
