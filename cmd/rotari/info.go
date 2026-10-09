package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type infoReport struct {
	MasterDir      string           `json:"masterdir"`
	BaseDir        string           `json:"basedir"`
	Project        string           `json:"project,omitempty"`
	ProjectExists  bool             `json:"project_exists"`
	ProjectChoices []string         `json:"project_choices,omitempty"`
	VisibleConfigs infoConfigFiles  `json:"visible_config_files"`
	Supervisors    []infoSupervisor `json:"running_supervisors"`
	RunLocks       []infoRunLock    `json:"run_locks"`
	ActiveRuns     []infoRun        `json:"active_runs"`
}

type infoConfigFiles struct {
	Common   []string            `json:"common,omitempty"`
	Projects map[string][]string `json:"projects,omitempty"`
}

type infoSupervisor struct {
	Project string `json:"project"`
	PID     int    `json:"pid"`
}

type infoRunLock struct {
	Project     string          `json:"project"`
	State       state.LockState `json:"state"`
	RunID       string          `json:"run_id,omitempty"`
	PID         int             `json:"pid,omitempty"`
	Host        string          `json:"host,omitempty"`
	Coordinator *bool           `json:"coordinator_alive,omitempty"`
}

type infoRun struct {
	Project string           `json:"project"`
	RunID   string           `json:"run_id"`
	Phase   project.RunPhase `json:"phase"`
	Jobs    *infoJobLiveness `json:"jobs,omitempty"`
}

type infoJobLiveness struct {
	Finished int `json:"finished"`
	// Failed counts the finished jobs whose result is a failure.
	Failed  int `json:"failed"`
	Pending int `json:"pending"`
	Alive   int `json:"alive"`
	Gone    int `json:"gone"`
	Unknown int `json:"unknown"`
}

func cmdInfo(args []string) int {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	baseDirFlag := cliString(fs, "basedir", "")
	projectFlag := cliString(fs, "project-name", "")
	masterDirFlag := cliString(fs, "masterdir", "")
	jsonOutput := cliBool(fs, "json", false)
	if err := cliParse(fs, args); err != nil || len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("info"))
		return 1
	}

	baseDir, _, err := state.ResolveBaseDir(*baseDirFlag)
	if err != nil {
		printErrorf("failed to resolve basedir: %v", err)
		return 1
	}
	masterDir, err := state.ResolveMasterDir(*masterDirFlag)
	if err != nil {
		printErrorf("failed to resolve masterdir: %v", err)
		return 1
	}
	selectedProject := *projectFlag
	if selectedProject == "" {
		selectedProject = configString("project-name", "")
	}
	if selectedProject == "" {
		selectedProject, err = configProjectName(baseDir, "")
		if err != nil {
			printErrorf("failed to resolve project: %v", err)
			return 1
		}
	}
	if selectedProject != "" && !state.IsValidPathElement(selectedProject) {
		printErrorf("invalid project name %q", selectedProject)
		return 1
	}

	common, projectConfigs := config.ListPaths(baseDir, selectedProject, notification.FileName)
	projects, err := infoProjects(baseDir)
	if err != nil {
		printErrorf("failed to list projects in %q: %v", baseDir, err)
		return 1
	}
	choices := []string(nil)
	if selectedProject == "" {
		choices = projects
	}
	projectExists := containsInfoProject(projects, selectedProject)
	report := infoReport{
		MasterDir:      masterDir,
		BaseDir:        baseDir,
		Project:        selectedProject,
		ProjectExists:  projectExists,
		ProjectChoices: choices,
		VisibleConfigs: infoConfigFiles{Common: common, Projects: projectConfigs},
	}
	if report.Supervisors, err = infoSupervisors(baseDir, projects); err != nil {
		printErrorf("failed to inspect supervisors: %v", err)
		return 1
	}
	if report.RunLocks, report.ActiveRuns, err = infoRuns(baseDir, projects); err != nil {
		printErrorf("failed to inspect runs: %v", err)
		return 1
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			printErrorf("failed to encode info: %v", err)
			return 1
		}
		return 0
	}
	printInfo(report)
	return 0
}

func infoProjects(baseDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	projects := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && state.IsValidPathElement(entry.Name()) {
			projects = append(projects, entry.Name())
		}
	}
	sort.Strings(projects)
	return projects, nil
}

func containsInfoProject(projects []string, name string) bool {
	index := sort.SearchStrings(projects, name)
	return name != "" && index < len(projects) && projects[index] == name
}

func infoSupervisors(baseDir string, projects []string) ([]infoSupervisor, error) {
	supervisors := make([]infoSupervisor, 0)
	for _, name := range projects {
		paths, err := state.ResolveProjectPaths(baseDir, name)
		if err != nil {
			continue
		}
		if pid, running := serverinternal.Running(paths.ProjectDir); running {
			supervisors = append(supervisors, infoSupervisor{Project: name, PID: pid})
		}
	}
	return supervisors, nil
}

func infoRuns(baseDir string, projects []string) ([]infoRunLock, []infoRun, error) {
	locks := make([]infoRunLock, 0)
	runs := make([]infoRun, 0)
	for _, name := range projects {
		paths, err := state.ResolveProjectPaths(baseDir, name)
		if err != nil {
			continue
		}
		lock, err := inspectInfoRunLock(name, paths)
		if err != nil {
			return nil, nil, err
		}
		if lock != nil {
			locks = append(locks, *lock)
		}
		projectRuns, err := infoProjectRuns(name, paths)
		if err != nil {
			return nil, nil, err
		}
		runs = append(runs, projectRuns...)
	}
	return locks, runs, nil
}

func inspectInfoRunLock(name string, paths state.ProjectPaths) (*infoRunLock, error) {
	lockState, lock, err := state.InspectLock(paths.LockFile, false)
	if err != nil {
		return nil, fmt.Errorf("project %q: %w", name, err)
	}
	if lockState == state.LockNone {
		return nil, nil
	}
	var coordinatorAlive *bool
	if lockState == state.LockActive || lockState == state.LockStale {
		alive := lockState == state.LockActive && state.ProcessAlive(lock.PID)
		coordinatorAlive = &alive
	}
	return &infoRunLock{
		Project: name, State: lockState, RunID: lock.RunID,
		PID: lock.PID, Host: lock.Host, Coordinator: coordinatorAlive,
	}, nil
}

func infoProjectRuns(name string, paths state.ProjectPaths) ([]infoRun, error) {
	entries, err := os.ReadDir(paths.RunsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list runs for project %q: %w", name, err)
	}
	runs := make([]infoRun, 0)
	for _, entry := range entries {
		run, active, err := inspectInfoRun(name, paths, entry)
		if err != nil {
			return nil, err
		}
		if active {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func inspectInfoRun(name string, paths state.ProjectPaths, entry os.DirEntry) (infoRun, bool, error) {
	if !entry.IsDir() || !state.IsValidPathElement(entry.Name()) {
		return infoRun{}, false, nil
	}
	phase, err := project.RunPhaseOf(paths, entry.Name())
	if err != nil {
		return infoRun{}, false, fmt.Errorf("inspect run %q in project %q: %w", entry.Name(), name, err)
	}
	if phase != project.RunPhaseRunning && phase != project.RunPhaseInterrupted {
		return infoRun{}, false, nil
	}
	runDir, err := state.SafeJoin(paths.RunsDir, entry.Name())
	if err != nil {
		return infoRun{}, false, err
	}
	jobs, err := infoRunJobLiveness(runDir)
	if err != nil {
		return infoRun{}, false, fmt.Errorf("inspect jobs in run %q for project %q: %w", entry.Name(), name, err)
	}
	return infoRun{Project: name, RunID: entry.Name(), Phase: phase, Jobs: jobs}, true, nil
}

func infoRunJobLiveness(runDir string) (*infoJobLiveness, error) {
	context, err := state.LoadContext(jsonStore(), runDir)
	hostKnown := err == nil && context.Hostname != ""
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	attemptDirs, err := state.LatestAttemptDirs(runDir)
	if err != nil {
		return nil, err
	}
	carried := jobstatus.RecordedResults(runDir, nil)
	result := &infoJobLiveness{}
	for _, carriedResult := range carried {
		result.countFinished(jobstatus.ResolveJob(jobstatus.Attempt{}, carriedResult, true))
	}
	for _, attemptDir := range attemptDirs {
		attempt := jobstatus.ReadAttempt(jsonStore(), attemptDir)
		if attempt.Finished() {
			var finalResult model.JobResult
			if jsonStore().ReadJSON(filepath.Join(attemptDir, state.FinalResultFileName), &finalResult) == nil {
				result.countFinished(jobstatus.ResolveJob(attempt, finalResult, true))
			} else {
				// A finished attempt without a final result awaits a retry.
				result.Pending++
			}
			continue
		}
		if !hostKnown || context.Hostname != hostname || state.ReadAttemptExecutor(attemptDir) != "local" {
			result.Unknown++
			continue
		}
		alive, known := executor.LocalProcessGroupAlive(attemptDir)
		switch {
		case !known:
			result.Unknown++
		case alive:
			result.Alive++
		default:
			result.Gone++
		}
	}
	result.Pending += infoNotStartedJobs(runDir, carried)
	if result.Finished+result.Pending+result.Alive+result.Gone+result.Unknown == 0 {
		return nil, nil
	}
	return result, nil
}

// countFinished counts a job with a final result, and whether it failed.
func (liveness *infoJobLiveness) countFinished(job jobstatus.Job) {
	liveness.Finished++
	if runview.LineageStatus(job) == runlineage.StatusFailed {
		liveness.Failed++
	}
}

// infoNotStartedJobs counts the run's jobs that have no attempt and no
// carried result.
func infoNotStartedJobs(runDir string, carried map[string]model.JobResult) int {
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return 0
	}
	count := 0
	for _, job := range model.QueueToJobs(queue.Commands) {
		if _, ok := carried[job.ID]; ok {
			continue
		}
		jobDir, err := state.LatestAttemptJobDir(runDir, job.ID)
		if err != nil {
			continue
		}
		if jobstatus.ReadJob(jsonStore(), jobDir, model.JobResult{}, false).DisplayStatus(job) == jobstatus.StatusPending {
			count++
		}
	}
	return count
}

func printInfo(report infoReport) {
	printInfoLocation(report)
	printInfoConfigs(report)
	printInfoSupervisors(report.Supervisors)
	printInfoRunLocks(report.RunLocks)
	printInfoRuns(report.ActiveRuns)
}

func printInfoLocation(report infoReport) {
	fmt.Printf("%s %s\n%s %s\n", cyan("Masterdir:"), report.MasterDir, cyan("Basedir:  "), report.BaseDir)
	if report.Project == "" {
		fmt.Printf("%s %s\n", cyan("Project:  "), yellow("(not selected)"))
		if len(report.ProjectChoices) > 0 {
			fmt.Printf("  %s %s\n", cyan("Available:"), strings.Join(report.ProjectChoices, ", "))
		}
	} else {
		fmt.Printf("%s %s", cyan("Project:  "), report.Project)
		if !report.ProjectExists {
			fmt.Printf(" %s", yellow("(not created)"))
		}
		fmt.Println()
	}
	fmt.Println()
}

func printInfoConfigs(report infoReport) {
	fmt.Println(cyan("Config files visible:"))
	if len(report.VisibleConfigs.Common) == 0 && len(report.VisibleConfigs.Projects) == 0 {
		fmt.Println("  (none found)")
	}
	for _, path := range report.VisibleConfigs.Common {
		fmt.Printf("  %s\n", path)
	}
	projectNames := make([]string, 0, len(report.VisibleConfigs.Projects))
	for name := range report.VisibleConfigs.Projects {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)
	for _, name := range projectNames {
		for _, path := range report.VisibleConfigs.Projects[name] {
			fmt.Printf("  %s (%s)\n", path, name)
		}
	}
	fmt.Println()
}

func printInfoSupervisors(supervisors []infoSupervisor) {
	fmt.Println(cyan("Running supervisors:"))
	if len(supervisors) == 0 {
		fmt.Println(infoNone)
	}
	for _, supervisor := range supervisors {
		fmt.Printf("  project=%s pid=%d\n", supervisor.Project, supervisor.PID)
	}
	fmt.Println()
}

func printInfoRunLocks(locks []infoRunLock) {
	fmt.Println(cyan("Run locks:"))
	if len(locks) == 0 {
		fmt.Println(infoNone)
	}
	for _, lock := range locks {
		alive := yellow("unknown")
		if lock.Coordinator != nil {
			if *lock.Coordinator {
				alive = green("true")
			} else {
				alive = red("false")
			}
		}
		fmt.Printf("  project=%s run=%s state=%s pid=%d host=%s coordinator_alive=%s\n", lock.Project, lock.RunID, infoLockState(lock.State), lock.PID, lock.Host, alive)
	}
	fmt.Println()
}

func printInfoRuns(runs []infoRun) {
	fmt.Println(cyan("Active or interrupted runs:"))
	if len(runs) == 0 {
		fmt.Println(infoNone)
	}
	for _, run := range runs {
		fmt.Printf("  project=%s run=%s phase=%s", run.Project, run.RunID, infoRunPhase(run.Phase))
		if run.Jobs != nil {
			fmt.Printf(" jobs=%s:%s %s:%s %s:%s %s:%s %s:%s %s:%s", cyan("finished"), cyan(fmt.Sprint(run.Jobs.Finished)), red("failed"), red(fmt.Sprint(run.Jobs.Failed)), cyan("pending"), cyan(fmt.Sprint(run.Jobs.Pending)), green("alive"), green(fmt.Sprint(run.Jobs.Alive)), red("gone"), red(fmt.Sprint(run.Jobs.Gone)), yellow("unknown"), yellow(fmt.Sprint(run.Jobs.Unknown)))
		}
		fmt.Println()
	}
}

func infoLockState(lockState state.LockState) string {
	switch lockState {
	case state.LockActive:
		return green(string(lockState))
	case state.LockStale:
		return red(string(lockState))
	default:
		return yellow(string(lockState))
	}
}

func infoRunPhase(phase project.RunPhase) string {
	switch phase {
	case project.RunPhaseRunning:
		return yellow(string(phase))
	case project.RunPhaseInterrupted:
		return red(string(phase))
	default:
		return green(string(phase))
	}
}

const infoNone = "  none"
