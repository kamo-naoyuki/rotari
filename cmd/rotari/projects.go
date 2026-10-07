package main

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func showAllProjects(cliBaseDir, cliMasterDir string) int {
	if cliBaseDir != "" {
		baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
		if err != nil {
			printErrorf("failed to resolve state directory: %v", err)
			return 1
		}
		return showProjectsForBaseDirs([]string{baseDir})
	}
	masterDir, err := state.ResolveMasterDir(cliMasterDir)
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	servers, err := listServers(masterDir)
	if err != nil {
		printErrorf("failed to list servers: %v", err)
		return 1
	}
	known, err := listKnownBaseDirs(masterDir, servers)
	if err != nil {
		printErrorf("failed to list known state directories: %v", err)
		return 1
	}
	baseDirs := make([]string, 0, len(known)+1)
	seen := make(map[string]bool, len(known)+1)
	for _, item := range known {
		seen[item.BaseDir] = true
		baseDirs = append(baseDirs, item.BaseDir)
	}
	if current, err := state.ResolveBaseDirDefault(); err == nil && !seen[current] {
		baseDirs = append(baseDirs, current)
	}
	sort.Strings(baseDirs)
	return showProjectsForBaseDirs(baseDirs)
}

func showProjectsForBaseDirs(baseDirs []string) int {
	fmt.Printf("%s\n", cyan("=== PROJECTS ==="))

	type projectInfo struct {
		baseDir    string
		name       string
		queued     int
		runs       int
		state      string
		lastRun    string
		lastResult string
	}
	projects := make([]projectInfo, 0)
	for _, baseDir := range baseDirs {
		overviews, err := project.Overviews(baseDir, true)
		if err != nil {
			printError(err)
			return 1
		}
		for _, overview := range overviews {
			lastRun := firstNonEmpty(overview.LastRun.ID, "-")
			projects = append(projects, projectInfo{baseDir: baseDir, name: overview.Name, queued: overview.Queued, runs: overview.Runs,
				state: projectStateName(overview.State), lastRun: lastRun, lastResult: lastRunResult(overview)})
		}
	}

	if len(projects) == 0 {
		fmt.Println("No projects found.")
		return 0
	}
	fmt.Printf("\n%s\n", cyan(fmt.Sprintf("Projects: %d", len(projects))))
	fmt.Println(cyan(fmt.Sprintf("%-36s %-24s %-8s %-8s %-14s %-24s %s", "BASEDIR", "PROJECT", "QUEUED", "RUNS", "STATE", "LAST RUN", "LAST RESULT")))
	// Without -b, `show -p` resolves the default state directory, so the
	// hint names the basedir when a listed project lives elsewhere.
	defaultBaseDir, _, defaultErr := state.ResolveBaseDir("")
	otherBaseDir, hasRun := false, false
	for _, project := range projects {
		fmt.Printf("%-36s %-24s %-8d %-8d %-14s %-24s %s\n", project.baseDir, project.name, project.queued, project.runs, project.state, project.lastRun, project.lastResult)
		otherBaseDir = otherBaseDir || defaultErr != nil || filepath.Clean(project.baseDir) != filepath.Clean(defaultBaseDir)
		hasRun = hasRun || project.lastRun != "-"
	}
	fmt.Println("\n" + cyan("To inspect a project:"))
	if otherBaseDir {
		fmt.Println("  rotari show -b BASEDIR -p PROJECT")
	} else {
		fmt.Println("  rotari show -p PROJECT")
	}
	if hasRun {
		fmt.Println(cyan("To summarize a run's failures by cause:"))
		if otherBaseDir {
			fmt.Println("  rotari lineage -b BASEDIR RUN_ID")
		} else {
			fmt.Println("  rotari lineage RUN_ID")
		}
	}
	return 0
}

// lastRunResult describes a project's last run for the project list:
// "running" while it runs, its status, with failed and total job counts once
// any job failed, or "-" without a readable summary.
func lastRunResult(overview project.Overview) string {
	last := overview.LastRun
	switch {
	case overview.State == project.Running:
		return "running"
	case last.Status == "":
		return "-"
	case last.Failed == 0:
		return last.Status
	}
	return fmt.Sprintf("%s %d/%d", last.Status, last.Failed, last.Jobs)
}
