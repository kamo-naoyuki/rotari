package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func confirmQueueOverwrite(baseDir, queueName string, appendJobs, overwriteJobs bool) (bool, error) {
	overwriteConfirmed := overwriteJobs
	if !appendJobs && !overwriteJobs {
		paths, pathErr := state.ResolveProjectPaths(baseDir, queueName)
		if pathErr != nil {
			return false, pathErr
		}
		queue, loadErr := state.LoadQueue(paths.QueueFile)
		if loadErr != nil {
			return false, loadErr
		}
		if len(queue.Commands) > 0 {
			if !stdinIsTerminal() {
				return false, errors.New("queue is not empty; use --append or --overwrite")
			}
			fmt.Fprintf(os.Stderr, "project %q has %d queued jobs; overwrite them? [y/N] ", queueName, len(queue.Commands))
			answer, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
			if readErr != nil && len(answer) == 0 {
				return false, readErr
			}
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				return false, errors.New("copy cancelled")
			}
			overwriteConfirmed = true
		}
	}
	return overwriteConfirmed, nil
}

// cmdCopy copies queued or historical jobs into the current project queue.
func cmdCopy(args []string) int {
	fs := flag.NewFlagSet("copy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	jobName := cliString(fs, "job-name", "")
	failed := cliBool(fs, "failed", false)
	unfinished := cliBool(fs, "unfinished", false)
	success := cliBool(fs, "success", false)
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	stage := cliString(fs, "stage", "")
	matrixName := cliString(fs, "matrix", "")
	appendJobs := cliBool(fs, "append", false)
	overwriteJobs := cliBool(fs, "overwrite", false)
	quiet := cliBool(fs, "quiet", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && *runID != "") || (*appendJobs && *overwriteJobs) {
		printError("usage: " + cliUsage("copy"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*runID = fs.Args()[0]
	}
	if *jobName != "" && len(jobIDs) > 0 {
		printError("--job-name cannot be combined with --job-id")
		return 1
	}
	scope := model.CommandSelector{Stage: *stage, Matrix: *matrixName}
	if scope.Kinds() > 1 || (scope.Kinds() > 0 && (*jobName != "" || len(jobIDs) > 0)) {
		printError("--stage, --matrix, and --job-id or --job-name cannot be combined")
		return 1
	}
	selection := model.ResultSelection(*failed, *unfinished, *success)
	if selection != "" && (*jobName != "" || len(jobIDs) > 0) {
		printError(errJobsWithResultFilter)
		return 1
	}
	if *jobName != "" {
		if *runID != "" {
			baseDir, projectName, resolvedRunID, err := resolve.ExistingRunID(*basedir, *queueNameOption, *runID)
			if err != nil {
				printError(err)
				return 1
			}
			*runID = resolvedRunID
			paths, err := state.ResolveProjectPaths(baseDir, projectName)
			if err != nil {
				printError(err)
				return 1
			}
			target, found, err := resolve.JobInRun(paths, *runID, *jobName, true)
			if err != nil || !found {
				printErrorf("job name %q not found in run %q", *jobName, *runID)
				return 1
			}
			jobIDs = stringSliceFlag{target.JobID}
		} else {
			targets, err := resolve.Jobs(*basedir, *queueNameOption, *jobName, true, false)
			if err != nil {
				printError(err)
				return 1
			}
			if len(targets) == 0 {
				printErrorf("job name %q not found", *jobName)
				return 1
			}
			if len(targets) > 1 {
				printError(resolve.AmbiguousError(fmt.Sprintf("job name %q", *jobName), targets))
				return 1
			}
			*basedir, *queueNameOption, *runID = targets[0].BaseDir, targets[0].ProjectName, targets[0].RunID
			jobIDs = stringSliceFlag{targets[0].JobID}
		}
	}
	if *runID == "" {
		for _, jobID := range jobIDs {
			if payload, err := state.DecodeAttemptID(jobID); err == nil {
				if *runID == "" {
					*runID = payload.RunID
				} else if *runID != payload.RunID {
					printError(fmt.Sprintf("attempt %q belongs to run %q, not %q", jobID, payload.RunID, *runID))
					return 1
				}
			}
		}
		if *runID == "" {
			if len(jobIDs) == 0 {
				baseDir, projectName, err := resolve.ExistingRun(*basedir, *queueNameOption, "")
				if err != nil {
					printError(err)
					return 1
				}
				paths, err := state.ResolveProjectPaths(baseDir, projectName)
				if err != nil {
					printError(err)
					return 1
				}
				meta, err := state.LoadMeta(paths.MetaFile)
				if err != nil {
					printErrorf("failed to load metadata: %v", err)
					return 1
				}
				if meta.LastRunID == "" {
					printErrorf("project %q has no previous run", projectName)
					return 1
				}
				*basedir, *queueNameOption, *runID = baseDir, projectName, meta.LastRunID
			} else {
				target, err := resolve.LatestJobIDs(*basedir, *queueNameOption, jobIDs)
				if err != nil {
					printError(err)
					return 1
				}
				*basedir, *queueNameOption, *runID = target.BaseDir, target.ProjectName, target.RunID
			}
		}
	}

	if len(jobIDs) > 0 {
		selection = "job-id"
	}
	if selection == "" {
		selection = "all"
	}

	baseDir, queueName, resolvedRunID, err := resolve.ExistingRunID(*basedir, *queueNameOption, *runID)
	if err != nil {
		printError(err)
		return 1
	}
	*runID = resolvedRunID
	if err := ensureProjectIdle(baseDir, queueName, "copy"); err != nil {
		printError(err)
		return 1
	}
	overwriteConfirmed, err := confirmQueueOverwrite(baseDir, queueName, *appendJobs, *overwriteJobs)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := queueEditor().Copy(baseDir, queueName, *runID, queueedit.CopyRequest{
		Selection: selection, JobIDs: jobIDs, Scope: scope, Append: *appendJobs, Overwrite: overwriteConfirmed,
	})
	if err != nil {
		printError(err)
		return 1
	}
	if !*quiet {
		fmt.Println(colorKeyValueMessage(message, green))
	}
	return 0
}
