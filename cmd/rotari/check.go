package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type projectCheck struct {
	State       string
	Runnable    bool
	RunID       string
	Queued      int
	QueuedKnown bool
	Lock        string
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectNameOption := cliString(fs, "project-name", "")
	jsonOutput := cliBool(fs, "json", false)
	deep := cliBool(fs, "deep", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("check"))
		return 1
	}

	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	projectName, err := resolveProjectName(baseDir, *projectNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := resolvePaths(baseDir, projectName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	result, err := checkProjectWithOptions(paths, *deep)
	if err != nil {
		printErrorf("failed to check project: %v", err)
		return 1
	}

	if err := writeProjectCheck(os.Stdout, projectName, result, *jsonOutput); err != nil {
		printErrorf("failed to print project check: %v", err)
		return 1
	}
	if result.Runnable {
		return 0
	}
	return 1
}

func writeProjectCheck(writer io.Writer, projectName string, result projectCheck, jsonOutput bool) error {
	if jsonOutput {
		var queued *int
		if result.QueuedKnown {
			queued = &result.Queued
		}
		return json.NewEncoder(writer).Encode(struct {
			Project  string `json:"project"`
			State    string `json:"state"`
			Runnable bool   `json:"runnable"`
			Queued   *int   `json:"queued"`
			Lock     string `json:"lock"`
			RunID    string `json:"run_id,omitempty"`
		}{projectName, result.State, result.Runnable, queued, result.Lock, result.RunID})
	}

	fmt.Fprintf(writer, "project=%s state=%s runnable=%t queued=", projectName, result.State, result.Runnable)
	if result.QueuedKnown {
		fmt.Fprintf(writer, "%d", result.Queued)
	} else {
		fmt.Fprint(writer, "unknown")
	}
	fmt.Fprintf(writer, " lock=%s", result.Lock)
	if result.RunID != "" {
		fmt.Fprintf(writer, " run_id=%s", result.RunID)
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func checkProject(paths pathSet) (projectCheck, error) {
	return checkProjectWithOptions(paths, false)
}

func checkProjectWithOptions(paths pathSet, deep bool) (projectCheck, error) {
	release, err := acquireStateReadLock(paths.stateLockFile)
	if err != nil {
		return projectCheck{}, fmt.Errorf("lock project state: %w", err)
	}
	defer release()

	inspection, err := inspectConsistentProjectState(paths, false)
	if err != nil {
		return projectCheck{}, fmt.Errorf("inspect project state: %w", err)
	}
	result := projectCheck{Lock: string(inspection.Lock), RunID: inspection.RunID}
	switch inspection.State {
	case projectRunning:
		if inspection.Lock == projectLockRemote {
			result.State = "locked"
		} else {
			result.State = "running"
		}
		loadQueueCount(paths, &result)
		return result, nil
	case projectInterrupted:
		result.State = "interrupted"
		loadQueueCount(paths, &result)
		return result, nil
	}

	queue, err := loadRunQueue(paths, "", nil, nil)
	if err != nil {
		return projectCheck{}, fmt.Errorf("validate queue: %w", err)
	}
	result.Queued = len(queue.Commands)
	result.QueuedKnown = true

	if result.Queued == 0 {
		result.State = "empty"
		return result, nil
	}
	if deep {
		if err := validateLocalExecutionEnvironment(queue); err != nil {
			return projectCheck{}, fmt.Errorf("validate execution environment: %w", err)
		}
	}
	result.State = "ready"
	result.Runnable = true
	return result, nil
}

func loadQueueCount(paths pathSet, result *projectCheck) {
	queue, err := loadQueue(paths.queueFile)
	if err != nil {
		return
	}
	result.Queued = len(queue.Commands)
	result.QueuedKnown = true
}
