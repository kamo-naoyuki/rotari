package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
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
	message, err := copyRunJobs(baseDir, queueName, *runID, queueedit.CopyRequest{
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

func copyRunToQueue(baseDir, queueName, runID, selection string, jobIDs []string, appendJobs bool, overwriteJobs ...bool) (string, error) {
	overwrite := len(overwriteJobs) > 0 && overwriteJobs[0]
	return copyRunJobs(baseDir, queueName, runID, queueedit.CopyRequest{Selection: selection, JobIDs: jobIDs, Append: appendJobs, Overwrite: overwrite})
}

// copyRunJobs copies the jobs that request selects from runID into the
// queue. Attempt IDs in request.JobIDs select those attempts.
func copyRunJobs(baseDir, queueName, runID string, request queueedit.CopyRequest) (string, error) {
	selection := request.Selection
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	var copied int
	err = project.EditQueue(paths, "copy", func(queue *model.Queue) error {
		sourceRunDir, err := state.SafeJoin(paths.RunsDir, runID)
		if err != nil {
			return err
		}
		snapshot, err := state.LoadQueue(filepath.Join(sourceRunDir, "commands.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
		if err != nil {
			return fmt.Errorf("failed to load command snapshot: %w", err)
		}
		if len(snapshot.Commands) == 0 {
			return errors.New("command snapshot has no jobs")
		}
		summary, summaryErr := state.LoadRunSummary(filepath.Join(sourceRunDir, "summary.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
		if summaryErr != nil && selection != "all" {
			return fmt.Errorf("failed to load run summary: %w", summaryErr)
		}
		source := queueedit.Run{
			ID: runID, Snapshot: snapshot, Results: model.ResultsByID(summary.Results),
			Timestamps: func(jobID string) (string, string) {
				return state.ReadJobTimestamp(sourceRunDir, jobID, stateFileSubmittedAt), state.ReadJobTimestamp(sourceRunDir, jobID, stateFileFinishedAt)
			},
		}
		if context, contextErr := state.LoadContext(jsonStore(), sourceRunDir); contextErr == nil {
			source.CWD = context.CWD
		}
		sourceRequest := request
		sourceRequest.JobIDs = nil
		for _, jobID := range request.JobIDs {
			if !strings.HasPrefix(jobID, "att_") {
				sourceRequest.JobIDs = append(sourceRequest.JobIDs, jobID)
				continue
			}
			attempt, err := copyAttempt(sourceRunDir, runID, jobID)
			if err != nil {
				return err
			}
			sourceRequest.Attempts = append(sourceRequest.Attempts, attempt)
		}
		edited, count, err := queueedit.Copy(*queue, queueName, source, sourceRequest, makeJobID)
		if err != nil {
			return err
		}
		*queue, copied = edited, count
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("copied jobs=%d from run=%s to queue=%s", copied, runID, queueName), nil
}

// copyAttempt checks that attemptID is an existing attempt of runID.
func copyAttempt(runDir, runID, attemptID string) (queueedit.Attempt, error) {
	payload, err := state.DecodeAttemptID(attemptID)
	if err != nil {
		return queueedit.Attempt{}, err
	}
	if payload.RunID != runID {
		return queueedit.Attempt{}, fmt.Errorf("attempt %q belongs to run %q, not %q", attemptID, payload.RunID, runID)
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, payload.JobID, attemptID)
	if err != nil {
		return queueedit.Attempt{}, err
	}
	// codeql[go/path-injection]: attemptDir is produced by validated attempt path helpers.
	if info, statErr := os.Stat(attemptDir); statErr != nil || !info.IsDir() {
		return queueedit.Attempt{}, fmt.Errorf("attempt %q not found in run %s", attemptID, runID)
	}
	return queueedit.Attempt{ID: attemptID, JobID: payload.JobID}, nil
}
