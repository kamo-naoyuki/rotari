package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdCancel cancels the active run or selected running jobs through the
// background server.
func cmdCancel(args []string) int {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	wait := cliBool(fs, "wait", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 && len(jobIDs) > 0 {
		printError("usage: " + cliUsage("cancel"))
		return 1
	}
	if len(fs.Args()) > 0 {
		jobIDs = append(jobIDs, fs.Args()...)
	}
	if len(jobIDs) > 0 && *wait {
		printError("--wait may not be used with a job selection")
		return 1
	}
	target, err := resolve.JobSelection(*basedir, *queueNameOption, jobIDs)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureServer(target.BaseDir); err != nil {
		printError(err)
		return 1
	}
	response, err := serverinternal.SendRequest(target.BaseDir, serverinternal.Request{Op: serverinternal.OpCancel, QueueName: target.ProjectName, RunID: target.RunID, JobIDs: target.JobIDs, Wait: *wait})
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Println(response.Message)
	return 0
}

// cmdJobSignal sends suspend or resume requests for selected running jobs.
func cmdJobSignal(args []string, operation string) int {
	fs := flag.NewFlagSet(operation, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 && len(jobIDs) > 0 {
		printError("usage: " + cliUsage(operation))
		return 1
	}
	if len(fs.Args()) > 0 {
		jobIDs = append(jobIDs, fs.Args()...)
	}
	target, err := resolve.JobSelection(*basedir, *queueNameOption, jobIDs)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureServer(target.BaseDir); err != nil {
		printError(err)
		return 1
	}
	response, err := serverinternal.SendRequest(target.BaseDir, serverinternal.Request{Op: operation, QueueName: target.ProjectName, RunID: target.RunID, JobIDs: target.JobIDs})
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Println(response.Message)
	return 0
}

func jobController() jobcontrol.Controller {
	return jobcontrol.Controller{Store: jsonStore(), Executors: executorRegistry}
}

func cancelQueue(baseDir, queueName string, wait bool) (string, error) {
	return cancelQueueJobs(baseDir, queueName, "", nil, wait)
}

// cancelQueueJobs cancels jobIDs, or the whole run when empty, in the active
// run of queueName, which must be runID when runID is not empty.
func cancelQueueJobs(baseDir, queueName, runID string, jobIDs []string, wait bool) (string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	return jobController().Cancel(paths, queueName, runID, jobIDs, wait)
}

// controlQueueJobs suspends or resumes jobIDs, or every running job when
// empty, in the active run of queueName, which must be runID when runID is
// not empty.
func controlQueueJobs(baseDir, queueName, runID string, jobIDs []string, operation string) (string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	return jobController().Control(paths, queueName, runID, jobIDs, operation)
}
