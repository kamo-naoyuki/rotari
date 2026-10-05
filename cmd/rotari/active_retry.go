package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	runpkg "github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type activeRetryOptions struct {
	baseDir, projectName, requestedRunID, jobName string
	jobIDs                                        []string
	selection                                     string
	scope                                         model.CommandSelector
	filter                                        jobfilter.Filter
	partialArray, dryRun                          bool
	ifRevision                                    string
	quiet                                         bool
}

type activeRetryTarget struct {
	baseDir, projectName, runID string
	paths                       state.ProjectPaths
}

// tryActiveRetry handles retry requests against a running project. handled is
// false only when the selected project has no active run, so runJobs can use
// its idle/new-run path. Once this path is selected it never falls back.
func tryActiveRetry(fs *flag.FlagSet, options activeRetryOptions) (exitCode int, handled bool) {
	target, active, err := findActiveRetryTarget(options)
	if err != nil {
		printError(err)
		return 1, true
	}
	if !active {
		return 0, false
	}
	if options.requestedRunID != "" && options.requestedRunID != target.runID && options.requestedRunID != "latest" {
		printErrorf("run %q is not the active run of project %q; active run is %q", options.requestedRunID, target.projectName, target.runID)
		return 1, true
	}
	jobIDs, selection, err := normalizeActiveRetryJobs(target, options)
	if err != nil {
		printError(err)
		return 1, true
	}
	if err := rejectActiveRetryOptions(fs, options.jobIDs, options.jobName, options.filter); err != nil {
		printError(err)
		return 1, true
	}
	controller := jobcontrol.Controller{Store: jsonStore(), Executors: executorRegistry}
	selectionRequest := jobcontrol.RetrySelection{
		JobIDs: jobIDs, Selection: selection, Scope: options.scope, Filter: options.filter, PartialArray: options.partialArray,
	}
	if options.dryRun {
		return previewActiveRetry(controller, target, selectionRequest, options.ifRevision), true
	}
	response, err := controller.SubmitSelectedRetry(target.baseDir, target.projectName, target.runID, selectionRequest, options.ifRevision, 30*time.Second)
	if err != nil {
		printError(err)
		return 1, true
	}
	return reportActiveRetry(target.runID, response, options.quiet), true
}

func previewActiveRetry(controller jobcontrol.Controller, target activeRetryTarget, selection jobcontrol.RetrySelection, ifRevision string) int {
	runID, selected, err := controller.SelectRetry(target.baseDir, target.projectName, target.runID, selection, time.Now())
	if err != nil {
		printError(err)
		return 1
	}
	fmt.Printf("dry run: retry active run=%s would reopen %d job(s)\n", runID, len(selected))
	for _, id := range selected {
		fmt.Printf("  retry job_id=%s\n", id)
	}
	if ifRevision != "" {
		if _, err := project.CheckRevision(target.paths, project.Guard{IfRevision: ifRevision}); err != nil {
			printError(err)
			return 1
		}
	}
	return 0
}

func reportActiveRetry(runID string, response runpkg.ManualRetryResponse, quiet bool) int {
	if response.RunEnded {
		printErrorf("run %q ended before accepting the retry request; repeating retry starts a new retry run", runID)
		return 1
	}
	if !quiet || len(response.Rejected) > 0 {
		fmt.Printf("retry accepted run_id=%s accepted=%d rejected=%d\n", runID, len(response.Accepted), len(response.Rejected))
		for _, id := range response.Accepted {
			fmt.Printf("  accepted job_id=%s\n", id)
		}
		for _, rejected := range response.Rejected {
			fmt.Printf("  rejected job_id=%s reason=%s\n", rejected.JobID, rejected.Reason)
		}
	}
	if len(response.Rejected) > 0 {
		return 1
	}
	return 0
}

func findActiveRetryTarget(options activeRetryOptions) (activeRetryTarget, bool, error) {
	baseDir, _, err := state.ResolveBaseDir(options.baseDir)
	if err != nil {
		return activeRetryTarget{}, false, err
	}
	projectName, err := state.ResolveProjectName(baseDir, options.projectName)
	if err != nil {
		return activeRetryTarget{}, false, err
	}
	if !resolve.ProjectExists(baseDir, projectName) {
		return activeRetryTarget{}, false, nil
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return activeRetryTarget{}, false, err
	}
	inspection, err := project.InspectConsistent(paths, true)
	if err != nil {
		return activeRetryTarget{}, false, fmt.Errorf("failed to check project state: %w", err)
	}
	if inspection.State != project.Running {
		return activeRetryTarget{}, false, nil
	}
	return activeRetryTarget{baseDir: baseDir, projectName: projectName, runID: inspection.RunID, paths: paths}, true, nil
}

func normalizeActiveRetryJobs(target activeRetryTarget, options activeRetryOptions) ([]string, string, error) {
	jobIDs := append([]string(nil), options.jobIDs...)
	for index, rawID := range jobIDs {
		if !strings.HasPrefix(rawID, "att_") {
			continue
		}
		payload, err := state.DecodeAttemptID(rawID)
		if err != nil {
			return nil, "", err
		}
		if payload.RunID != target.runID {
			return nil, "", fmt.Errorf("attempt %q belongs to run %q, not active run %q", rawID, payload.RunID, target.runID)
		}
		jobIDs[index] = payload.JobID
	}
	if options.jobName != "" {
		job, found, err := resolve.JobInRun(target.paths, target.runID, options.jobName, true)
		if err != nil {
			return nil, "", err
		}
		if !found {
			return nil, "", fmt.Errorf("job name %q not found in active run %q", options.jobName, target.runID)
		}
		jobIDs = append(jobIDs, job.JobID)
	}
	selection := options.selection
	if len(jobIDs) > 0 && selection == "" {
		selection = "job-id"
	}
	return jobIDs, selection, nil
}

func rejectActiveRetryOptions(fs *flag.FlagSet, jobIDs []string, jobName string, filter jobfilter.Filter) error {
	if !filter.Empty() && (filter.Changed || filter.New) {
		return fmt.Errorf("--filter-changed and --filter-new cannot select jobs for an active-run retry")
	}
	if optionSupplied(fs, "async") {
		return fmt.Errorf("--async cannot be used for retry inside an active run")
	}
	unsupported := []string{"executor", "executor-option", "env", "local-concurrency", "batch-concurrency", "retry", "run-name", "match-by", "ssh-concurrency", "ssh-options", "slurm-concurrency", "slurm-options", "slurm-submit-interval", "slurm-submit-retry-limit", "pbs-concurrency", "pbs-options", "pbs-submit-interval", "pbs-submit-retry-limit", "lsf-concurrency", "lsf-options", "lsf-submit-interval", "lsf-submit-retry-limit", "sge-concurrency", "sge-options", "sge-submit-interval", "sge-submit-retry-limit"}
	for _, name := range unsupported {
		if optionSupplied(fs, name) {
			return fmt.Errorf("--%s cannot be changed by retry inside an active run", name)
		}
	}
	if jobName != "" && len(jobIDs) > 0 {
		return errors.New("--job-name cannot be combined with --job-id")
	}
	return nil
}

func optionSupplied(fs *flag.FlagSet, name string) bool {
	if cliOptionSet(fs, name) {
		return true
	}
	if _, ok := configValue(name); ok {
		return true
	}
	if envName := cliEnvironmentVariable(name); envName != "" {
		_, ok := os.LookupEnv(envName)
		return ok
	}
	return false
}
