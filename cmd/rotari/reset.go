package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdReset clears interrupted project state so the queue can be edited or run
// again.
func cmdReset(args []string) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	recoverOption := cliBool(fs, "recover", false)
	quiet := cliBool(fs, "quiet", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("reset"))
		return 1
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := resolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	projectState, runID, err := inspectConsistentProjectRunState(paths, true)
	if err != nil {
		printErrorf("failed to check project state: %v", err)
		return 1
	}
	if projectState == projectRunning {
		meta, metaErr := state.LoadMeta(paths.MetaFile)
		if metaErr == nil && meta.Phase == "cancelling" {
			if !waitForCancellation(paths, queueName) {
				return 1
			}
			projectState, runID, err = inspectConsistentProjectRunState(paths, true)
			if err != nil {
				printErrorf("failed to check project state: %v", err)
				return 1
			}
		}
	}
	if projectState == projectRunning {
		fmt.Fprint(os.Stderr, formatProjectRunningError(paths, runID))
		return 1
	}
	if projectState == projectInterrupted {
		confirmed := *recoverOption
		if !confirmed {
			if !isTerminal(os.Stdin) {
				detail, stillRunning := interruptedRunStatusDetail(paths, runID)
				message := fmt.Sprintf("project %q has interrupted run %q%s; reset requires confirmation\nInspect before deciding: rotari show --basedir %s --project-name %s --run-id %s\n",
					queueName, runID, detail, executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
				if stillRunning {
					message += "Do not recover until you have independently confirmed those jobs have actually stopped.\n"
				}
				message += fmt.Sprintf("Confirm with:\n  rotari reset --basedir %s --project-name %s --recover",
					executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName))
				printError(message)
				return 1
			}
			confirmed, err = confirmResetOfInterruptedRun(os.Stdin, os.Stderr, paths, runID)
			if err != nil {
				printErrorf("failed to confirm reset: %v", err)
				return 1
			}
			if !confirmed {
				printError("reset cancelled")
				return 1
			}
		}
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			printErrorf("failed to load queue: %v", err)
			return 1
		}
		cleared := len(queue.Commands)
		if err := recoverInterruptedProject(paths, runID, true); err != nil {
			printErrorf("failed to recover interrupted run: %v", err)
			return 1
		}
		if !*quiet {
			fmt.Printf("%s\n", colorKeyValueMessage(fmt.Sprintf("reset project=%s cleared=%d job(s); recovered interrupted run=%s", queueName, cleared, runID), yellow))
		}
		return 0
	}
	cleared, err := resetQueueCommands(paths)
	if err != nil {
		printErrorf("failed to reset queue: %v", err)
		return 1
	}
	if *quiet {
		return 0
	}
	color := green
	if cleared > 0 {
		color = yellow
	}
	fmt.Printf("%s\n", colorKeyValueMessage(fmt.Sprintf("reset project=%s cleared=%d job(s)", queueName, cleared), color))
	return 0
}

func confirmResetOfInterruptedRun(input io.Reader, output io.Writer, paths pathSet, runID string) (bool, error) {
	detail, _ := interruptedRunStatusDetail(paths, runID)
	fmt.Fprintf(output, "project %q has interrupted run %q%s.\nConfirm all jobs have stopped and reset the queue? [y/N] ", paths.ProjectName, runID, detail)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

// resetQueueCommands preserves queue defaults and run history.
func resetQueueCommands(paths pathSet) (int, error) {
	if err := os.MkdirAll(paths.ProjectDir, stateDirMode()); err != nil {
		return 0, fmt.Errorf("failed to create project directory: %w", err)
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return 0, fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	running, err := isRunning(paths.LockFile)
	if err != nil {
		return 0, fmt.Errorf("failed to check queue: %w", err)
	}
	if running {
		return 0, fmt.Errorf("project %q is running; reset is not allowed", paths.ProjectName)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return 0, fmt.Errorf("failed to load queue: %w", err)
	}
	cleared := len(queue.Commands)
	if cleared > 0 {
		queue.Commands = nil
		if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
			return 0, fmt.Errorf("failed to reset queue: %w", err)
		}
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return 0, fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return 0, fmt.Errorf("failed to update metadata: %w", err)
	}
	return cleared, nil
}
