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
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/project"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type infoReport struct {
	MasterDir       string           `json:"masterdir"`
	BaseDir         string           `json:"basedir"`
	Project         string           `json:"project,omitempty"`
	ProjectChoices  []string         `json:"project_choices,omitempty"`
	LoadedConfigs   []config.Source  `json:"loaded_config_sources,omitempty"`
	VisibleConfigs  infoConfigFiles  `json:"visible_config_files"`
	Supervisors     []infoSupervisor `json:"running_supervisors"`
	RunLocks        []infoRunLock    `json:"run_locks"`
	ActiveRuns      []infoRun        `json:"active_runs"`
	JobLivenessNote string           `json:"job_liveness_note"`
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
	report := infoReport{
		MasterDir:       masterDir,
		BaseDir:         baseDir,
		Project:         selectedProject,
		ProjectChoices:  choices,
		LoadedConfigs:   append([]config.Source(nil), cliFileConfig.Sources...),
		VisibleConfigs:  infoConfigFiles{Common: common, Projects: projectConfigs},
		JobLivenessNote: "Individual job process liveness is not checked; job states are recorded results. Coordinator liveness is reported from the run lock.",
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
		if !entry.IsDir() || !state.IsValidPathElement(entry.Name()) {
			continue
		}
		phase, err := project.RunPhaseOf(paths, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("inspect run %q in project %q: %w", entry.Name(), name, err)
		}
		if phase == project.RunPhaseRunning || phase == project.RunPhaseInterrupted {
			runs = append(runs, infoRun{Project: name, RunID: entry.Name(), Phase: phase})
		}
	}
	return runs, nil
}

func printInfo(report infoReport) {
	printInfoLocation(report)
	printInfoConfigs(report)
	printInfoSupervisors(report.Supervisors)
	printInfoRunLocks(report.RunLocks)
	printInfoRuns(report.ActiveRuns)
	fmt.Printf("\nNote: %s\n", report.JobLivenessNote)
}

func printInfoLocation(report infoReport) {
	fmt.Printf("Masterdir: %s\nBasedir:   %s\n", report.MasterDir, report.BaseDir)
	if report.Project == "" {
		fmt.Println("Project:   (not selected)")
		if len(report.ProjectChoices) > 0 {
			fmt.Printf("  Available: %s\n", strings.Join(report.ProjectChoices, ", "))
		}
	} else {
		fmt.Printf("Project:   %s\n", report.Project)
	}
	fmt.Println()
}

func printInfoConfigs(report infoReport) {
	fmt.Println("Config files visible:")
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
	fmt.Println("\nLoaded config sources:")
	if len(report.LoadedConfigs) == 0 {
		fmt.Println("  (none)")
	}
	for _, source := range report.LoadedConfigs {
		fmt.Printf("  %s: %s\n", source.Scope, source.Path)
	}
	fmt.Println()
}

func printInfoSupervisors(supervisors []infoSupervisor) {
	fmt.Println("Running supervisors:")
	if len(supervisors) == 0 {
		fmt.Println(infoNone)
	}
	for _, supervisor := range supervisors {
		fmt.Printf("  project=%s pid=%d\n", supervisor.Project, supervisor.PID)
	}
	fmt.Println()
}

func printInfoRunLocks(locks []infoRunLock) {
	fmt.Println("Run locks:")
	if len(locks) == 0 {
		fmt.Println(infoNone)
	}
	for _, lock := range locks {
		alive := "unknown"
		if lock.Coordinator != nil {
			alive = fmt.Sprint(*lock.Coordinator)
		}
		fmt.Printf("  project=%s run=%s state=%s pid=%d host=%s coordinator_alive=%s\n", lock.Project, lock.RunID, lock.State, lock.PID, lock.Host, alive)
	}
	fmt.Println()
}

func printInfoRuns(runs []infoRun) {
	fmt.Println("Active or interrupted runs:")
	if len(runs) == 0 {
		fmt.Println(infoNone)
	}
	for _, run := range runs {
		fmt.Printf("  project=%s run=%s phase=%s\n", run.Project, run.RunID, run.Phase)
	}
}

const infoNone = "  none"
