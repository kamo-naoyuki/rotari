package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdWait waits for selected runs to finish and returns the final run exit code
// when a single run is targeted.
func cmdWait(args []string) int {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var explicitRunIDs stringSliceFlag
	cliValue(fs, &explicitRunIDs, "run-id")
	timeout := cliDuration(fs, "timeout", 0)
	jsonOutput := cliBool(fs, "json", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	selectors := fs.Args()
	targets := make([]waitTarget, 0, len(explicitRunIDs)+len(selectors))
	for _, runID := range explicitRunIDs {
		targets = append(targets, waitTarget{baseDir: *basedir, projectName: *queueNameOption, runID: runID})
	}
	for _, selector := range selectors {
		target, err := resolveWaitTarget(*basedir, *queueNameOption, selector)
		if err != nil {
			printError(err)
			return 1
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		activeTargets, err := resolveActiveWaitTargets(*basedir, *queueNameOption)
		if err != nil {
			printError(err)
			return 1
		}
		if len(activeTargets) == 0 {
			printError("no active runs")
			return 1
		}
		if len(activeTargets) > 1 {
			printError("multiple active runs; specify a project or selector:")
			for _, target := range activeTargets {
				fmt.Fprintf(os.Stderr, "  project=%s run=%s\n", target.projectName, target.runID)
			}
			return 1
		}
		targets = append(targets, activeTargets[0])
	}
	if *timeout < 0 {
		printError("--timeout must be >= 0")
		return 1
	}
	deadline := time.Time{}
	if *timeout > 0 {
		deadline = time.Now().Add(*timeout)
	}
	exitCode := 0
	for _, target := range targets {
		result := waitForRun(target.baseDir, target.projectName, target.runID, deadline, *jsonOutput)
		if result.exitCode > exitCode {
			exitCode = result.exitCode
		}
		if result.timedOut {
			return 1
		}
	}
	return exitCode
}

type waitTarget struct {
	baseDir     string
	projectName string
	runID       string
}

func resolveRunNameTargets(cliBaseDir, cliProjectName, runName string, activeOnly bool) ([]waitTarget, error) {
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return nil, err
	}
	projectNames, err := projectNamesForRunName(baseDir, cliProjectName)
	if err != nil {
		return nil, err
	}
	targets := make([]waitTarget, 0)
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
				targets = append(targets, waitTarget{baseDir: baseDir, projectName: projectName, runID: lock.RunID})
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
				targets = append(targets, waitTarget{baseDir: baseDir, projectName: projectName, runID: entry.Name()})
			}
		}
	}
	return targets, nil
}

func projectNamesForRunName(baseDir, cliProjectName string) ([]string, error) {
	if cliProjectName != "" || os.Getenv(envProjectName) != "" {
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

func resolveWaitTarget(cliBaseDir, cliProjectName, selector string) (waitTarget, error) {
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return waitTarget{}, err
	}
	if state.IsValidPathElement(selector) {
		projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), selector)
		if err == nil {
			if info, statErr := os.Stat(projectDir); statErr == nil && info.IsDir() {
				runID, activeErr := resolveActiveRunTarget(baseDir, selector)
				if activeErr != nil {
					return waitTarget{}, activeErr
				}
				return waitTarget{baseDir: baseDir, projectName: selector, runID: runID}, nil
			}
		}
	}

	activeTargets, err := resolveRunNameTargets(baseDir, cliProjectName, selector, true)
	if err != nil {
		return waitTarget{}, err
	}
	if len(activeTargets) == 1 {
		return activeTargets[0], nil
	}
	if len(activeTargets) > 1 {
		return waitTarget{}, fmt.Errorf("run name %q is ambiguous across active projects", selector)
	}
	if location, found, registryErr := resolveRunLocation(selector); registryErr != nil {
		return waitTarget{}, registryErr
	} else if found {
		return waitTarget{baseDir: location.BaseDir, projectName: location.ProjectName, runID: location.RunID}, nil
	}
	return waitTarget{}, fmt.Errorf("no project, active run name, or run ID matches %q", selector)
}

func resolveActiveWaitTargets(cliBaseDir, cliProjectName string) ([]waitTarget, error) {
	if cliProjectName != "" || os.Getenv(envProjectName) != "" {
		runID, err := resolveActiveRunTarget(cliBaseDir, cliProjectName)
		if err != nil {
			return nil, err
		}
		baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
		if err != nil {
			return nil, err
		}
		projectName, err := state.ResolveProjectName(baseDir, cliProjectName)
		if err != nil {
			return nil, err
		}
		return []waitTarget{{baseDir: baseDir, projectName: projectName, runID: runID}}, nil
	}
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	active := make([]waitTarget, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths, pathErr := state.ResolveProjectPaths(baseDir, entry.Name())
		if pathErr != nil {
			return nil, pathErr
		}
		running, runningErr := isRunning(paths.LockFile)
		if runningErr != nil {
			return nil, runningErr
		}
		if !running {
			continue
		}
		lock, lockErr := state.LoadLock(paths.LockFile)
		if lockErr != nil {
			return nil, lockErr
		}
		active = append(active, waitTarget{baseDir: baseDir, projectName: entry.Name(), runID: lock.RunID})
	}
	sort.Slice(active, func(i, j int) bool { return active[i].projectName < active[j].projectName })
	return active, nil
}

func resolveActiveRunTarget(cliBaseDir, cliProjectName string) (string, error) {
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return "", err
	}
	queueName, err := state.ResolveProjectName(baseDir, cliProjectName)
	if err != nil {
		return "", err
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	inspection, err := project.Inspect(paths, true)
	state, runID := inspection.State, inspection.RunID
	if err != nil {
		return "", fmt.Errorf("failed to check project state: %w", err)
	}
	if state != project.Running || runID == "" {
		return "", fmt.Errorf("project %q has no active run", queueName)
	}
	return runID, nil
}

type waitResult struct {
	exitCode int
	timedOut bool
}

func waitForRun(basedir, queueNameOption, runID string, deadline time.Time, jsonOutput bool) waitResult {
	baseDir, queueName, err := resolveExistingRunTarget(basedir, queueNameOption, runID)
	if err != nil {
		printError(err)
		return waitResult{exitCode: 1}
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return waitResult{exitCode: 1}
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printErrorf("invalid run ID %q", runID)
		return waitResult{exitCode: 1}
	}
	for {
		summaryPath, pathErr := state.ValidatedStateFile(runDir, "summary.json")
		if pathErr != nil {
			printErrorf("invalid run directory %q", runID)
			return waitResult{exitCode: 1}
		}
		summary, err := state.LoadRunSummary(summaryPath)
		if err == nil {
			if jsonOutput {
				_ = json.NewEncoder(os.Stdout).Encode(summary)
			} else {
				fmt.Print(formatRunCompletion(paths, runID, summary))
			}
			return waitResult{exitCode: summary.ExitCode}
		}
		if runInfo, statErr := os.Stat(runDir); os.IsNotExist(statErr) || (statErr == nil && !runInfo.IsDir()) {
			printErrorf("run %q is registered but its run directory is missing; run 'rotari gc' to inspect stale registry entries", runID)
			return waitResult{exitCode: 1}
		} else if statErr != nil {
			printErrorf("failed to inspect run directory %s: %v", runDir, statErr)
			return waitResult{exitCode: 1}
		}
		if errors.Is(err, os.ErrNotExist) {
			if message, ended := runEndedWithoutSummary(paths, runID, summaryPath); ended {
				printError(message)
				return waitResult{exitCode: 1}
			}
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			printErrorf("timed out waiting for run %s", runID)
			return waitResult{exitCode: 1, timedOut: true}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// runEndedWithoutSummary reports whether runID is no longer active although it
// never wrote summaryPath, for example because its supervisor exited early.
// It leaves a stale run lock in place for show, unlock, and reset --recover.
func runEndedWithoutSummary(paths state.ProjectPaths, runID, summaryPath string) (string, bool) {
	inspection, err := project.Inspect(paths, false)
	if err != nil || (inspection.State == project.Running && inspection.RunID == runID) {
		return "", false
	}
	// The run may have finished between the summary read and the state check.
	if _, err := os.Stat(summaryPath); !errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	target := fmt.Sprintf("--basedir %s --project-name %s --run-id %s",
		executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
	if inspection.State == project.Interrupted && inspection.RunID == runID {
		return fmt.Sprintf("run %s was interrupted before it wrote a summary; inspect it with 'rotari show %s', then recover with 'rotari unlock %s'", runID, target, target), true
	}
	return fmt.Sprintf("run %s is not active and has no summary; inspect it with 'rotari show %s'", runID, target), true
}

func formatRunCompletion(paths state.ProjectPaths, runID string, summary model.RunSummary) string {
	successCount := 0
	failedCount := 0
	for _, result := range summary.Results {
		if result.ExitCode == 0 {
			successCount++
		} else {
			failedCount++
		}
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	title := "=== Run finished ==="
	if summary.ExitCode != 0 {
		title = red("=== Run failed ===")
	} else {
		title = green(title)
	}
	message := title + "\n" + colorLabeledDetails(fmt.Sprintf("  Project: %s\n  Run: %s\n  Status: %s\n  Exit code: %d\n  Success: %d\n  Failed: %d\n  Directory: %s\n",
		paths.ProjectName, formatRunLabel(runID, summary.RunName), summary.Status, summary.ExitCode, successCount, failedCount, runDir), summary.ExitCode != 0)
	if failedCount > 0 {
		message += colorLabeledDetails(fmt.Sprintf("\nInspect run:\n  rotari show --run-id %s\n\nSee failed job output below.\n\nFailed job output:\n%s\nRerun failed jobs:\n  rotari retry --basedir %s --project-name %s\n",
			runID, failedJobHints(runID, summary.Results), paths.BaseDir, paths.ProjectName), summary.ExitCode != 0)
	}
	return message
}
