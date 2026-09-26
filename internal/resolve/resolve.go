// Package resolve turns command-line selectors into the base directory,
// project, run, and job they name.
//
// It owns the location rules shared by every command that reads existing
// state: run IDs found through the run registry, attempt IDs, the latest-run
// fallback, run names, and job IDs or names looked up in the queue and the
// latest runs. Command-specific selector orders, such as show's or wait's,
// stay with the command and build on these functions.
package resolve

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// lookupRun finds runID in the default run registry.
func lookupRun(runID string) (runregistry.Location, bool, error) {
	registry, err := runregistry.Default()
	if err != nil {
		return runregistry.Location{}, false, err
	}
	return registry.Lookup(runID)
}

// IsRunID reports whether value has the shape of a generated run ID, telling
// a bare run ID apart from a job ID or an "att_" attempt ID.
func IsRunID(value string) bool {
	return runIDPattern.MatchString(value)
}

func isRunning(lockPath string) (bool, error) {
	lockState, _, err := state.InspectLock(lockPath, true)
	return lockState == state.LockActive || lockState == state.LockRemote, err
}

// runIDPattern matches the generated run ID shape: a UTC timestamp and a
// random hex suffix.
var runIDPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{8}$`)

// JobSelection resolves the base directory and project for a
// cancel/suspend/resume job selection that may mix plain job IDs, "att_"
// attempt IDs, and at most one bare run ID. A bare run ID only locates the
// target run through the run registry; it is not itself a job selector, so it
// is removed from the returned job IDs.
func JobSelection(cliBaseDir, cliProjectName string, ids []string) (string, string, []string, error) {
	targetRunID := ""
	jobIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		switch {
		case runIDPattern.MatchString(id):
			if targetRunID != "" && targetRunID != id {
				return "", "", nil, fmt.Errorf("selection mixes run %q and run %q", targetRunID, id)
			}
			targetRunID = id
		case strings.HasPrefix(id, "att_"):
			payload, err := state.DecodeAttemptID(id)
			if err != nil {
				return "", "", nil, err
			}
			if targetRunID != "" && targetRunID != payload.RunID {
				return "", "", nil, fmt.Errorf("attempt %q belongs to run %q, not %q", id, payload.RunID, targetRunID)
			}
			targetRunID = payload.RunID
			jobIDs = append(jobIDs, id)
		default:
			jobIDs = append(jobIDs, id)
		}
	}
	baseDir, queueName, err := ExistingRun(cliBaseDir, cliProjectName, targetRunID)
	if err != nil {
		return "", "", nil, err
	}
	return baseDir, queueName, jobIDs, nil
}

// SplitProjectOrRun classifies positional selectors that name either a
// project or a saved run ID. A selector with the generated run ID shape is a
// run ID; anything else is a project name. At most one project may be named.
func SplitProjectOrRun(selectors []string) (string, []string, error) {
	projectName := ""
	runIDs := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		if runIDPattern.MatchString(selector) {
			runIDs = append(runIDs, selector)
			continue
		}
		if projectName != "" && projectName != selector {
			return "", nil, fmt.Errorf("selection names project %q and project %q", projectName, selector)
		}
		projectName = selector
	}
	return projectName, runIDs, nil
}

// Attempt resolves the base directory, project, run, and job of an "att_"
// attempt ID. A cliRunID that disagrees with the attempt's run is an error.
func Attempt(attemptID, cliBaseDir, cliProjectName, cliRunID string) (string, string, string, string, error) {
	payload, err := state.DecodeAttemptID(attemptID)
	if err != nil {
		return "", "", "", "", err
	}
	if cliRunID != "" && cliRunID != payload.RunID {
		return "", "", "", "", fmt.Errorf("attempt %q belongs to run %q, not %q", attemptID, payload.RunID, cliRunID)
	}
	baseDir, projectName, err := ExistingRun(cliBaseDir, cliProjectName, payload.RunID)
	if err != nil {
		return "", "", "", "", err
	}
	return baseDir, projectName, payload.RunID, payload.JobID, nil
}

// ExistingRun resolves the base directory and project for a command that
// reads existing state. A registered runID supplies both; explicit values that
// conflict with its registry entry are errors, and a registered run whose
// directory is gone is reported as stale. Otherwise the normal base directory
// and project resolution applies.
func ExistingRun(cliBaseDir, cliProjectName, runID string) (string, string, error) {
	if runID != "" {
		location, found, err := lookupRun(runID)
		if err != nil {
			return "", "", err
		}
		if found {
			if cliBaseDir != "" {
				baseDir, err := filepath.Abs(cliBaseDir)
				if err != nil {
					return "", "", err
				}
				if baseDir != location.BaseDir {
					return "", "", fmt.Errorf("run %q is registered under basedir %q, not %q", runID, location.BaseDir, baseDir)
				}
			} else {
				cliBaseDir = location.BaseDir
			}
			if cliProjectName != "" && cliProjectName != location.ProjectName {
				return "", "", fmt.Errorf("run %q is registered under project %q, not %q", runID, location.ProjectName, cliProjectName)
			}
			if cliProjectName == "" {
				cliProjectName = location.ProjectName
			}
			if !location.Exists() {
				return "", "", fmt.Errorf("run %q is registered but its run directory is missing; run 'rotari gc' to inspect stale registry entries", runID)
			}
		}
	}
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return "", "", err
	}
	projectName, err := state.ResolveProjectName(baseDir, cliProjectName)
	if err != nil {
		return "", "", err
	}
	return baseDir, projectName, nil
}

// ExistingRunID is ExistingRun that also resolves runID to a saved run of the
// project: "latest" becomes the latest run, and any other ID must exist. An
// empty runID stays empty.
func ExistingRunID(cliBaseDir, cliProjectName, runID string) (string, string, string, error) {
	baseDir, projectName, err := ExistingRun(cliBaseDir, cliProjectName, runID)
	if err != nil || runID == "" {
		return baseDir, projectName, runID, err
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", "", "", err
	}
	resolved, err := RunID(paths, runID)
	if err != nil {
		return "", "", "", err
	}
	return baseDir, projectName, resolved, nil
}

// Run names a run's location.
type Run struct {
	BaseDir     string
	ProjectName string
	RunID       string
}

// RunsByName finds runs named runName in the selected projects: the active
// run of each project when activeOnly, otherwise every saved run.
func RunsByName(cliBaseDir, cliProjectName, runName string, activeOnly bool) ([]Run, error) {
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return nil, err
	}
	projectNames, err := ProjectNames(baseDir, cliProjectName)
	if err != nil {
		return nil, err
	}
	targets := make([]Run, 0)
	for _, projectName := range projectNames {
		paths, pathErr := state.ResolveProjectPaths(baseDir, projectName)
		if pathErr != nil {
			return nil, pathErr
		}
		if activeOnly {
			running, runningErr := isRunning(paths.LockFile)
			if runningErr != nil {
				if os.IsNotExist(runningErr) {
					continue
				}
				return nil, runningErr
			}
			if !running {
				continue
			}
			lock, lockErr := state.LoadLock(paths.LockFile)
			if lockErr != nil {
				return nil, lockErr
			}
			if lock.RunName == runName {
				targets = append(targets, Run{BaseDir: baseDir, ProjectName: projectName, RunID: lock.RunID})
			}
			continue
		}
		entries, readErr := os.ReadDir(paths.RunsDir)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, readErr
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			summary, summaryErr := state.LoadRunSummary(filepath.Join(paths.RunsDir, entry.Name(), "summary.json"))
			if summaryErr == nil && summary.RunName == runName {
				targets = append(targets, Run{BaseDir: baseDir, ProjectName: projectName, RunID: entry.Name()})
			}
		}
	}
	return targets, nil
}

// ProjectExists reports whether name is an existing project of baseDir, as
// positional selectors such as those of show and wait try first.
func ProjectExists(baseDir, name string) bool {
	if !state.IsValidPathElement(name) {
		return false
	}
	projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), name)
	if err != nil {
		return false
	}
	info, err := os.Stat(projectDir)
	return err == nil && info.IsDir()
}

// ProjectNames lists the projects a search covers: the explicitly chosen
// project, or every project in baseDir, sorted.
func ProjectNames(baseDir, cliProjectName string) ([]string, error) {
	if state.ProjectNameGiven(cliProjectName) {
		projectName, err := state.ResolveProjectName(baseDir, cliProjectName)
		if err != nil {
			return nil, err
		}
		return []string{projectName}, nil
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	projects := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			projects = append(projects, entry.Name())
		}
	}
	sort.Strings(projects)
	return projects, nil
}

// Candidate priorities for Jobs, best first.
const (
	priorityActive = iota
	priorityInterrupted
	priorityQueue
	priorityLatest
)

// Job names a job in a run, or in the project's queue when FromQueue is set.
type Job struct {
	Run
	JobID string
	// FromQueue reports a job found in the current queue rather than a run.
	FromQueue bool
	// priority orders candidates in Jobs: active, interrupted, queue, latest.
	priority int
}

// defaultJobs finds a job in one project without a run ID: in the active or
// interrupted run when there is one, otherwise in the queue and the latest run.
func defaultJobs(paths state.ProjectPaths, selector string, byName bool) ([]Job, error) {
	inspection, err := project.Inspect(paths, true)
	projectState, stateRunID := inspection.State, inspection.RunID
	if err != nil {
		return nil, err
	}
	if projectState == project.Running || projectState == project.Interrupted {
		target, found, err := JobInRun(paths, stateRunID, selector, byName)
		if err != nil || !found {
			return nil, err
		}
		target.priority = priorityActive
		if projectState == project.Interrupted {
			target.priority = priorityInterrupted
		}
		return []Job{target}, nil
	}
	targets := make([]Job, 0, 2)
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return nil, err
	}
	if len(queue.Commands) > 0 {
		jobID, found := JobInQueue(queue, selector, byName)
		if found {
			targets = append(targets, Job{
				Run:   Run{BaseDir: paths.BaseDir, ProjectName: paths.ProjectName},
				JobID: jobID, FromQueue: true, priority: priorityQueue,
			})
		}
	}
	runID, err := RunID(paths, "")
	if err != nil {
		return targets, nil
	}
	target, found, err := JobInRun(paths, runID, selector, byName)
	if err != nil || !found {
		return targets, err
	}
	target.priority = priorityLatest
	return append(targets, target), nil
}

// highestPriority keeps the candidates with the best priority.
func highestPriority(targets []Job) []Job {
	if len(targets) == 0 {
		return targets
	}
	bestPriority := targets[0].priority
	for _, target := range targets[1:] {
		if target.priority < bestPriority {
			bestPriority = target.priority
		}
	}
	best := make([]Job, 0, len(targets))
	for _, target := range targets {
		if target.priority == bestPriority {
			best = append(best, target)
		}
	}
	return best
}

// JobInQueue finds a job by ID, or by name when byName, in queue. A command's
// own ID or name wins, so an array job's ID or name returns the array's
// command ID; a task ID or name, such as "ID-2" or "NAME[2]", returns that
// task's ID.
func JobInQueue(queue model.Queue, selector string, byName bool) (string, bool) {
	for _, command := range queue.Commands {
		if (byName && command.Name != "" && command.Name == selector) || (!byName && command.ID == selector) {
			return command.ID, true
		}
	}
	for _, job := range model.QueueToJobs(queue.Commands) {
		if (byName && job.Name == selector) || (!byName && job.ID == selector) {
			return job.ID, true
		}
	}
	return "", false
}

// JobInRun finds a job by ID or name in runID's command snapshot. A run
// without a readable snapshot has no jobs.
func JobInRun(paths state.ProjectPaths, runID, selector string, byName bool) (Job, bool, error) {
	queue, err := state.LoadQueue(filepath.Join(paths.RunsDir, runID, "commands.json"))
	if err != nil {
		return Job{}, false, nil
	}
	jobID, found := JobInQueue(queue, selector, byName)
	if !found {
		return Job{}, false, nil
	}
	return Job{
		Run:   Run{BaseDir: paths.BaseDir, ProjectName: paths.ProjectName, RunID: runID},
		JobID: jobID,
	}, true, nil
}

// Jobs finds a job by ID or name across the selected projects. With
// includeQueue it applies defaultJobs to each project and keeps the best
// candidates; otherwise it looks only in each project's latest run.
func Jobs(cliBaseDir, cliProjectName, selector string, byName, includeQueue bool) ([]Job, error) {
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return nil, err
	}
	projects, err := ProjectNames(baseDir, cliProjectName)
	if err != nil {
		return nil, err
	}
	targets := make([]Job, 0)
	for _, projectName := range projects {
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			return nil, err
		}
		if includeQueue {
			projectTargets, err := defaultJobs(paths, selector, byName)
			if err != nil {
				return nil, err
			}
			targets = append(targets, projectTargets...)
			continue
		}
		runID, err := RunID(paths, "")
		if err != nil {
			continue
		}
		target, found, err := JobInRun(paths, runID, selector, byName)
		if err != nil {
			return nil, err
		}
		if found {
			target.priority = priorityLatest
			targets = append(targets, target)
		}
	}
	if includeQueue {
		return highestPriority(targets), nil
	}
	return targets, nil
}

// LatestJobIDs finds the one latest run that contains every job ID. It fails
// when an ID is missing, ambiguous, or the IDs span different runs.
func LatestJobIDs(cliBaseDir, cliProjectName string, jobIDs []string) (Job, error) {
	return jobIDsTarget(cliBaseDir, cliProjectName, jobIDs, false)
}

// QueuedOrLatestJobIDs finds where every job ID is, like LatestJobIDs, but
// prefers a project's non-empty queue to its latest run, as Jobs does with
// includeQueue.
func QueuedOrLatestJobIDs(cliBaseDir, cliProjectName string, jobIDs []string) (Job, error) {
	return jobIDsTarget(cliBaseDir, cliProjectName, jobIDs, true)
}

func jobIDsTarget(cliBaseDir, cliProjectName string, jobIDs []string, includeQueue bool) (Job, error) {
	where := "latest runs"
	if includeQueue {
		where = "queues or latest runs"
	}
	var target Job
	for index, jobID := range jobIDs {
		targets, err := Jobs(cliBaseDir, cliProjectName, jobID, false, includeQueue)
		if err != nil {
			return Job{}, err
		}
		if len(targets) == 0 {
			return Job{}, fmt.Errorf("job %q not found in %s", jobID, where)
		}
		if len(targets) > 1 {
			return Job{}, fmt.Errorf("job %q is ambiguous across %s", jobID, where)
		}
		if index == 0 {
			target = targets[0]
		} else if targets[0].Run != target.Run || targets[0].FromQueue != target.FromQueue {
			return Job{}, fmt.Errorf("job IDs resolve to different %s", where)
		}
	}
	return target, nil
}

// RunID resolves the run a history command reads. An explicit requested run
// must exist; empty or "latest" selects meta.json's last run, then the newest
// run directory.
func RunID(paths state.ProjectPaths, requested string) (string, error) {
	if requested == model.Latest {
		requested = ""
	}
	if requested != "" {
		if !state.IsValidPathElement(requested) {
			return "", fmt.Errorf("run %q not found", requested)
		}
		// codeql[go/path-injection]: requested is validated by IsValidPathElement.
		if _, err := os.Stat(filepath.Join(paths.RunsDir, requested)); err != nil {
			return "", fmt.Errorf("run %q not found", requested)
		}
		return requested, nil
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	if meta.LastRunID != "" {
		// codeql[go/path-injection]: LastRunID is loaded from validated state metadata.
		if info, err := os.Stat(filepath.Join(paths.RunsDir, meta.LastRunID)); err == nil && info.IsDir() {
			return meta.LastRunID, nil
		}
	}
	{
		entries, err := os.ReadDir(paths.RunsDir)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("project %q has no runs (runs_dir=%s)", paths.ProjectName, paths.RunsDir)
			}
			return "", fmt.Errorf("failed to read runs: %w", err)
		}
		runIDs := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				runIDs = append(runIDs, entry.Name())
			}
		}
		if len(runIDs) == 0 {
			return "", fmt.Errorf("project %q has no runs (runs_dir=%s)", paths.ProjectName, paths.RunsDir)
		}
		sort.Slice(runIDs, func(i, j int) bool {
			// codeql[go/path-injection]: runIDs come from directory entries under RunsDir.
			left, _ := os.Stat(filepath.Join(paths.RunsDir, runIDs[i]))
			// codeql[go/path-injection]: runIDs come from directory entries under RunsDir.
			right, _ := os.Stat(filepath.Join(paths.RunsDir, runIDs[j]))
			return left.ModTime().After(right.ModTime())
		})
		return runIDs[0], nil
	}
}
