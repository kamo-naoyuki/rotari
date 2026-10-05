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
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/supervisor"
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
	untilFailure := cliBool(fs, "until-failure", false)
	jsonOutput := cliBool(fs, "json", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if *timeout < 0 {
		printError("--timeout must be >= 0")
		return 1
	}
	selectors := fs.Args()
	targets := make([]resolve.Run, 0, len(explicitRunIDs)+len(selectors))
	for _, runID := range explicitRunIDs {
		targets = append(targets, resolve.Run{BaseDir: *basedir, ProjectName: *queueNameOption, RunID: runID})
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
				fmt.Fprintf(os.Stderr, "  project=%s run=%s\n", target.ProjectName, target.RunID)
			}
			return 1
		}
		targets = append(targets, activeTargets[0])
	}
	deadline := time.Time{}
	if *timeout > 0 {
		deadline = time.Now().Add(*timeout)
	}
	exitCode := 0
	for _, target := range targets {
		if target.RunID == "" {
			continue // A project that does not exist yet has nothing to wait for.
		}
		result := waitForRun(target.BaseDir, target.ProjectName, target.RunID, deadline, *untilFailure, *jsonOutput)
		if result.exitCode > exitCode {
			exitCode = result.exitCode
		}
		if result.timedOut {
			return 1
		}
	}
	return exitCode
}

func resolveWaitTarget(cliBaseDir, cliProjectName, selector string) (resolve.Run, error) {
	if selector == model.Latest {
		baseDir, projectName, runID, err := resolve.ExistingRunID(cliBaseDir, cliProjectName, selector)
		if err != nil {
			return resolve.Run{}, err
		}
		return resolve.Run{BaseDir: baseDir, ProjectName: projectName, RunID: runID}, nil
	}
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return resolve.Run{}, err
	}
	// A project or run name waits for its active run, or else returns the
	// latest matching run's result, so a run that ends before wait is called
	// is not an error.
	if resolve.ProjectExists(baseDir, selector) {
		return resolveProjectWaitTarget(baseDir, selector)
	}
	selectedProject := cliProjectName
	if selectedProject == "" {
		selectedProject = os.Getenv(envProjectName)
	}
	if selectedProject == selector && !resolve.IsRunID(selector) && state.IsValidPathElement(selector) {
		return resolve.Run{BaseDir: baseDir, ProjectName: selector}, nil
	}

	activeTargets, err := resolve.RunsByName(baseDir, cliProjectName, selector, true)
	if err != nil {
		return resolve.Run{}, err
	}
	if len(activeTargets) == 0 {
		named, err := resolve.RunsByName(baseDir, cliProjectName, selector, false)
		if err != nil {
			return resolve.Run{}, err
		}
		activeTargets = latestRunPerProject(named)
	}
	if len(activeTargets) == 1 {
		return activeTargets[0], nil
	}
	if len(activeTargets) > 1 {
		candidates := make([]resolve.Job, 0, len(activeTargets))
		for _, target := range activeTargets {
			candidates = append(candidates, resolve.Job{Run: target})
		}
		return resolve.Run{}, resolve.AmbiguousError(fmt.Sprintf("run name %q", selector), candidates)
	}
	if _, found, registryErr := resolveRunLocation(selector); registryErr != nil {
		return resolve.Run{}, registryErr
	} else if found {
		// A run ID locates its own run, and an explicit base directory or
		// project that disagrees with the registry is an error, as with
		// --run-id and every other command.
		baseDir, projectName, err := resolve.ExistingRun(cliBaseDir, cliProjectName, selector)
		if err != nil {
			return resolve.Run{}, err
		}
		return resolve.Run{BaseDir: baseDir, ProjectName: projectName, RunID: selector}, nil
	}
	if state.IsValidPathElement(selector) && !resolve.IsRunID(selector) &&
		selectedProject == "" {
		return resolve.Run{BaseDir: baseDir, ProjectName: selector}, nil
	}
	return resolve.Run{}, fmt.Errorf("no project, run name, or run ID matches %q", selector)
}

// resolveProjectWaitTarget applies the same active-then-latest rule to a
// project selected by a positional argument, option, or environment variable.
func resolveProjectWaitTarget(baseDir, projectName string) (resolve.Run, error) {
	runID, err := resolveActiveRunTarget(baseDir, projectName)
	if err != nil {
		baseDir, projectName, runID, err = resolve.ExistingRunID(baseDir, projectName, model.Latest)
		if err != nil {
			return resolve.Run{}, err
		}
	}
	return resolve.Run{BaseDir: baseDir, ProjectName: projectName, RunID: runID}, nil
}

// latestRunPerProject keeps the newest of runs in each project. Run IDs
// start with their creation time, so the greatest ID is the newest.
func latestRunPerProject(runs []resolve.Run) []resolve.Run {
	latest := make(map[string]resolve.Run)
	var order []string
	for _, run := range runs {
		key := run.BaseDir + "\x00" + run.ProjectName
		current, seen := latest[key]
		if !seen {
			order = append(order, key)
		}
		if !seen || run.RunID > current.RunID {
			latest[key] = run
		}
	}
	result := make([]resolve.Run, 0, len(order))
	for _, key := range order {
		result = append(result, latest[key])
	}
	return result
}

func resolveActiveWaitTargets(cliBaseDir, cliProjectName string) ([]resolve.Run, error) {
	if cliProjectName != "" || os.Getenv(envProjectName) != "" {
		baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
		if err != nil {
			return nil, err
		}
		projectName, err := state.ResolveProjectName(baseDir, cliProjectName)
		if err != nil {
			return nil, err
		}
		if !resolve.ProjectExists(baseDir, projectName) {
			return []resolve.Run{{BaseDir: baseDir, ProjectName: projectName}}, nil
		}
		target, err := resolveProjectWaitTarget(baseDir, projectName)
		if err != nil {
			return nil, err
		}
		return []resolve.Run{target}, nil
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
	active := make([]resolve.Run, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths, pathErr := state.ResolveProjectPaths(baseDir, entry.Name())
		if pathErr != nil {
			return nil, pathErr
		}
		lockState, _, lockErr := state.InspectLock(paths.LockFile, false)
		if lockErr != nil {
			return nil, lockErr
		}
		if lockState != state.LockActive && lockState != state.LockRemote {
			continue
		}
		lock, lockErr := state.LoadLock(paths.LockFile)
		if lockErr != nil {
			return nil, lockErr
		}
		active = append(active, resolve.Run{BaseDir: baseDir, ProjectName: entry.Name(), RunID: lock.RunID})
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ProjectName < active[j].ProjectName })
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
	if err := resolve.RequireProject(baseDir, queueName); err != nil {
		return "", err
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	inspection, err := project.Inspect(paths, false)
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

// waitForRun waits until runID finishes, or with untilFailure until one of
// its jobs has failed with no retry left, whichever comes first.
func waitForRun(basedir, queueNameOption, runID string, deadline time.Time, untilFailure, jsonOutput bool) waitResult {
	baseDir, queueName, runID, err := resolve.ExistingRunID(basedir, queueNameOption, runID)
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
			phase, phaseErr := project.RunPhaseOf(paths, runID)
			if phaseErr != nil {
				printErrorf("failed to check project state: %v", phaseErr)
				return waitResult{exitCode: 1}
			}
			if phase == project.RunPhaseRunning {
				if !deadline.IsZero() && time.Now().After(deadline) {
					printErrorf("timed out waiting for run %s", runID)
					return waitResult{exitCode: 1, timedOut: true}
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}
			if phase == project.RunPhaseInterrupted {
				printErrorf("run %s was interrupted after writing its summary; inspect it with 'rotari show --basedir %s --project-name %s --run-id %s', then recover with 'rotari unlock --basedir %s --project-name %s --run-id %s'",
					runID, executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID),
					executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
				return waitResult{exitCode: 1}
			}
			if jsonOutput {
				_ = json.NewEncoder(os.Stdout).Encode(summary)
			} else {
				fmt.Print(formatRunCompletion(paths, runID, summary))
			}
			return waitResult{exitCode: summary.ExitCode}
		}
		if runInfo, statErr := os.Stat(runDir); os.IsNotExist(statErr) || (statErr == nil && !runInfo.IsDir()) {
			printErrorf("run %q is registered but its run directory is missing; run 'rotari gc --dry-run' to list stale registry entries and 'rotari gc' to remove them", runID)
			return waitResult{exitCode: 1}
		} else if statErr != nil {
			printErrorf("failed to inspect run directory %s: %v", runDir, statErr)
			return waitResult{exitCode: 1}
		}
		if errors.Is(err, os.ErrNotExist) {
			if message, ended := runEndedWithoutSummary(paths, runID); ended {
				printError(message)
				return waitResult{exitCode: 1}
			}
		} else if errors.Is(err, state.ErrNewerStateVersion) {
			printError(err)
			return waitResult{exitCode: 1}
		} else if errors.Is(err, state.ErrInvalidJSON) {
			if message, ended := runEndedWithInvalidSummary(paths, runID); ended {
				printError(message)
				return waitResult{exitCode: 1}
			}
		} else {
			printErrorf("failed to read run summary %s: %v", summaryPath, err)
			return waitResult{exitCode: 1}
		}
		if untilFailure {
			if failures := finalFailureGroups(paths, runID); len(failures) > 0 {
				writeEarlyFailures(paths, runID, failures, jsonOutput)
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

// finalFailureGroups groups the jobs of an active run that have failed with
// no retry left; see runview.FinalFailureGroups. A run that cannot be read
// yet has none.
func finalFailureGroups(paths state.ProjectPaths, runID string) []runlineage.FailureGroup {
	groups, err := runview.FinalFailureGroups(paths, runID, jsonStore())
	if err != nil {
		return nil
	}
	return groups
}

// earlyFailureJSON is what wait --until-failure --json prints for a run that
// is still running when a job has failed.
type earlyFailureJSON struct {
	RunID    string                    `json:"run_id"`
	Status   string                    `json:"status"`
	Failures []runlineage.FailureGroup `json:"failures"`
}

// writeEarlyFailures reports that runID, still running, has failed jobs.
func writeEarlyFailures(paths state.ProjectPaths, runID string, failures []runlineage.FailureGroup, jsonOutput bool) {
	if jsonOutput {
		_ = json.NewEncoder(os.Stdout).Encode(earlyFailureJSON{RunID: runID, Status: "running", Failures: failures})
		return
	}
	fmt.Println(red(fmt.Sprintf("Run %s is still running, and jobs have failed.", runID)))
	// The run is still running, so a retry of its failures cannot start yet.
	writeFailureGroups(os.Stdout, failures, nil)
	target := fmt.Sprintf("--basedir %s --project-name %s --run-id %s",
		executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
	fmt.Println(cyan("To keep waiting:"))
	fmt.Printf("  rotari wait %s\n", target)
	fmt.Println(cyan("To cancel the run:"))
	fmt.Printf("  rotari cancel --basedir %s --project-name %s\n", executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName))
}

// runEndedWithInvalidSummary reports an invalid summary only after its run is
// no longer active, allowing wait to tolerate a summary being atomically replaced.
func runEndedWithInvalidSummary(paths state.ProjectPaths, runID string) (string, bool) {
	phase, err := project.RunPhaseOf(paths, runID)
	if err != nil || phase == project.RunPhaseRunning || phase == project.RunPhaseFinished {
		return "", false
	}
	target := fmt.Sprintf("--basedir %s --project-name %s --run-id %s",
		executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
	if phase == project.RunPhaseInterrupted {
		return fmt.Sprintf("run %s was interrupted without a valid summary; inspect it with 'rotari show %s', then recover with 'rotari unlock %s'", runID, target, target), true
	}
	return fmt.Sprintf("run %s is not active and has no valid summary; inspect it with 'rotari show %s'", runID, target), true
}

// runEndedWithoutSummary reports whether runID is no longer active although it
// never wrote summaryPath, for example because its supervisor exited early.
// It leaves a stale run lock in place for show and unlock.
func runEndedWithoutSummary(paths state.ProjectPaths, runID string) (string, bool) {
	// RunPhaseOf reads the summary again, so a run that finished since the
	// caller's read is not reported as ended.
	phase, err := project.RunPhaseOf(paths, runID)
	if err != nil || phase == project.RunPhaseRunning || phase == project.RunPhaseFinished {
		return "", false
	}
	target := fmt.Sprintf("--basedir %s --project-name %s --run-id %s",
		executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
	if phase == project.RunPhaseInterrupted {
		return fmt.Sprintf("run %s was interrupted before it wrote a summary; inspect it with 'rotari show %s', then recover with 'rotari unlock %s'", runID, target, target), true
	}
	return fmt.Sprintf("run %s is not active and has no summary; inspect it with 'rotari show %s'", runID, target), true
}

func formatRunCompletion(paths state.ProjectPaths, runID string, summary model.RunSummary) string {
	return colorMessage(supervisor.CompletionMessage(paths, runID, summary))
}
