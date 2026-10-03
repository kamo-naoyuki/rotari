package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
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
	guard := cliGuardFlags(fs)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "project-name")) {
		printError("usage: " + cliUsage("reset"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*queueNameOption = fs.Args()[0]
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := state.ResolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	if !resolve.ProjectExists(baseDir, queueName) {
		if err := model.ValidateReservedName("project", queueName); err != nil {
			printError(err)
			return 1
		}
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	inspection, err := project.InspectConsistent(paths, !*guard.dryRun)
	projectState, runID := inspection.State, inspection.RunID
	if err != nil {
		printErrorf("failed to check project state: %v", err)
		return 1
	}
	if projectState == project.Running && !*guard.dryRun {
		meta, metaErr := state.LoadMeta(paths.MetaFile)
		if metaErr == nil && meta.Phase == "cancelling" {
			if !waitForCancellation(paths, queueName) {
				return 1
			}
			inspection, err = project.InspectConsistent(paths, true)
			projectState, runID = inspection.State, inspection.RunID
			if err != nil {
				printErrorf("failed to check project state: %v", err)
				return 1
			}
		}
	}
	if projectState == project.Running {
		fmt.Fprint(os.Stderr, formatProjectRunningError(paths, runID))
		return 1
	}
	if projectState == project.Interrupted {
		// A dry run changes nothing, so it needs no confirmation.
		confirmed := *recoverOption || *guard.dryRun
		if !confirmed {
			if !stdinIsTerminal() {
				detail, stillRunning := project.InterruptedRunDetail(paths, runID)
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
		if err := project.RecoverInterruptedGuarded(paths, runID, true, guard.guard()); err != nil {
			printErrorf("failed to recover interrupted run: %v", err)
			return 1
		}
		guard.printResult(fmt.Sprintf("reset project=%s cleared=%d job(s); recovered interrupted run=%s", queueName, cleared, runID), *quiet)
		return 0
	}
	cleared, err := resetQueueCommands(paths, guard.guard())
	if err != nil {
		printErrorf("failed to reset queue: %v", err)
		return 1
	}
	guard.printResult(fmt.Sprintf("reset project=%s cleared=%d job(s)", queueName, cleared), *quiet)
	return 0
}

func confirmResetOfInterruptedRun(input io.Reader, output io.Writer, paths state.ProjectPaths, runID string) (bool, error) {
	detail, _ := project.InterruptedRunDetail(paths, runID)
	fmt.Fprintf(output, "project %q has interrupted run %q%s.\nConfirm all jobs have stopped and reset the queue? [y/N] ", paths.ProjectName, runID, detail)
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

// resetQueueCommands empties the queue under guard, preserving its defaults
// and the run history. A dry run of a project that does not exist yet
// creates nothing.
func resetQueueCommands(paths state.ProjectPaths, guard project.Guard) (int, error) {
	if _, err := os.Stat(paths.ProjectDir); errors.Is(err, os.ErrNotExist) && guard.DryRun {
		revision, err := project.CheckRevision(paths, guard)
		if err != nil {
			return 0, err
		}
		if guard.Report != nil {
			guard.Report(project.Outcome{Revision: revision})
		}
		return 0, nil
	}
	if !guard.DryRun {
		if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
			return 0, fmt.Errorf("failed to create project directory: %w", err)
		}
	}
	cleared := 0
	err := project.EditQueueGuarded(paths, "reset", guard, func(queue *model.Queue) error {
		cleared = len(queue.Commands)
		queue.Commands = nil
		return nil
	})
	return cleared, err
}
