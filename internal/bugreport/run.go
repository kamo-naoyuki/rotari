package bugreport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rotari-dev/rotari/internal/executor"
	"github.com/rotari-dev/rotari/internal/report"
	"github.com/rotari-dev/rotari/internal/state"
	webprojection "github.com/rotari-dev/rotari/internal/web"
)

var runStateFiles = []string{
	"summary.json", "commands.json", "context.json", state.RunClientStatusFileName,
	state.CarriedResultsFileName, state.RunSourcesFileName, state.ProgressFileName,
	state.LoadSamplesFileName, state.RunNotesFileName, state.StaleLockRemovalFileName,
}

var attemptStateFiles = []string{
	"command.json", "job.json", "status.json", "status", "scheduler_status.json",
	state.SchedulerEventsFileName, "submitted_at", "finished_at", "pid", "cancelled",
	"name", state.FinalResultFileName, state.ArtifactsFileName,
}

var outputFiles = []string{state.StdoutFileName, state.StderrFileName, "output"}

func (collector *collector) run() error {
	options := collector.options
	paths, err := state.ResolveProjectPaths(options.BaseDir, options.Project)
	if err != nil {
		return err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, options.RunID)
	if err != nil {
		return fmt.Errorf("run %q not found", options.RunID)
	}
	if info, err := os.Stat(runDir); err != nil || !info.IsDir() {
		return fmt.Errorf("run %q not found in project %q", options.RunID, options.Project)
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	for _, name := range []string{filepath.Base(paths.MetaFile), filepath.Base(paths.LockFile), "server.pid"} {
		collector.addStateFile("project/"+name, filepath.Join(paths.ProjectDir, name))
	}
	prefix := "run/" + options.RunID + "/"
	for _, name := range runStateFiles {
		collector.addStateFile(prefix+name, filepath.Join(runDir, name))
	}

	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		collector.note("summary.json not read: %v", err)
	}
	jobs, err := webprojection.LoadRunJobs(store, runDir, "")
	if err != nil {
		collector.note("jobs of run %s not read: %v", options.RunID, err)
		return nil
	}
	selected, omitted, err := selectJobs(jobs, options.RunID, options.JobIDs)
	if err != nil {
		return err
	}

	builder := &collector.summary
	fmt.Fprintf(builder, "## Run %s\n\n", options.RunID)
	fmt.Fprintf(builder, "- Status: %s, exit code %d\n", valueOr(summary.Status, "unknown"), summary.ExitCode)
	fmt.Fprintf(builder, "- Started: %s; finished: %s\n", valueOr(summary.StartedAt, "-"), valueOr(summary.FinishedAt, "-"))
	selection := "jobs that did not succeed"
	if len(options.JobIDs) > 0 {
		selection = "jobs named with --job"
	}
	fmt.Fprintf(builder, "- Jobs: %d in the run; %d included (%s)\n", len(jobs), len(selected), selection)
	if omitted > 0 {
		fmt.Fprintf(builder, "- %d more jobs were left out; name them with --job to include them\n", omitted)
	}
	builder.WriteString("\n")
	if len(selected) > 0 {
		builder.WriteString("| Job | Name | Latest attempt | Status | Exit | Executor | Scheduler state | Error |\n|---|---|---|---|---|---|---|---|\n")
		for _, job := range selected {
			exitCode, errorText := "-", ""
			if job.Result != nil {
				exitCode = fmt.Sprint(job.Result.ExitCode)
				errorText = job.Result.Error
			}
			executorName := job.Executor
			if dir, err := state.LatestAttemptJobDir(runDir, job.ID); err == nil && executorName == "" {
				executorName = state.ReadAttemptExecutor(dir)
			}
			fmt.Fprintf(builder, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", tableCell(job.ID), tableCell(job.Name), tableCell(job.AttemptID), tableCell(job.ExecutionStatus), exitCode, tableCell(executorName), tableCell(job.SchedulerState), tableCell(errorText))
		}
		builder.WriteString("\n")
	}
	probes := maxProbedJobs
	for index, job := range selected {
		if job.Result != nil {
			for _, host := range job.Result.Hosts {
				collector.known[host] = "[REDACTED_HOST]"
			}
		}
		collector.job(store, runDir, prefix, job, index < summaryOutputJobs, &probes)
	}
	if probes == 0 {
		collector.note("scheduler queries were run for at most %d scheduler jobs", maxProbedJobs)
	}

	if text, err := report.Build(store, paths, options.RunID, "", true, "", options.Redact); err == nil {
		collector.addFile(prefix+"report.md", []byte(text))
	} else {
		collector.note("run report not built: %v", err)
	}
	return nil
}

// selectJobs returns the jobs named by ids, which may be job or attempt IDs,
// or else the jobs that did not succeed, up to maxJobs, and how many more
// there were.
func selectJobs(jobs []webprojection.Job, runID string, ids []string) ([]webprojection.Job, int, error) {
	if len(ids) == 0 {
		var selected []webprojection.Job
		for _, job := range jobs {
			if !strings.HasPrefix(job.ExecutionStatus, "success") {
				selected = append(selected, job)
			}
		}
		if len(selected) > maxJobs {
			return selected[:maxJobs], len(selected) - maxJobs, nil
		}
		return selected, 0, nil
	}
	byID := make(map[string]webprojection.Job, len(jobs))
	for _, job := range jobs {
		byID[job.ID] = job
	}
	seen := make(map[string]bool, len(ids))
	selected := make([]webprojection.Job, 0, len(ids))
	for _, id := range ids {
		jobID := id
		if payload, err := state.DecodeAttemptID(id); err == nil {
			jobID = payload.JobID
		}
		job, ok := byID[jobID]
		if !ok {
			return nil, 0, fmt.Errorf("job %q not found in run %q", id, runID)
		}
		if !seen[jobID] {
			seen[jobID] = true
			selected = append(selected, job)
		}
	}
	return selected, 0, nil
}

func (collector *collector) job(store state.Store, runDir, prefix string, job webprojection.Job, showOutput bool, probes *int) {
	builder := &collector.summary
	title := job.ID
	if job.Name != "" {
		title += " (" + job.Name + ")"
	}
	fmt.Fprintf(builder, "### Job %s\n\n", title)
	if !state.IsValidPathElement(job.ID) {
		collector.note("job %q skipped: invalid job ID", job.ID)
		return
	}
	attempts := state.ListAttemptIDs(runDir, job.ID)
	if len(attempts) == 0 {
		attempts = []string{""}
	} else {
		// A cancel that came before the job's first attempt records itself here.
		collector.addStateFile(prefix+"jobs/"+job.ID+"/job/cancelled", filepath.Join(runDir, job.ID, "cancelled"))
	}
	for index, attemptID := range attempts {
		latest := index == len(attempts)-1
		dir, label := filepath.Join(runDir, job.ID), "job"
		if attemptID != "" {
			var err error
			if dir, err = state.SpecificAttemptJobDir(runDir, job.ID, attemptID); err != nil {
				collector.note("attempt %q of job %s skipped: %v", attemptID, job.ID, err)
				continue
			}
			label = attemptID
		}
		dirPrefix := prefix + "jobs/" + job.ID + "/" + label + "/"
		files := make(map[string][]byte)
		for _, name := range attemptStateFiles {
			if data, ok := collector.addStateFile(dirPrefix+name, filepath.Join(dir, name)); ok {
				files[name] = data
			}
		}
		outputs := collector.jobOutputs(dirPrefix, dir)
		if !latest {
			continue
		}
		fmt.Fprintf(builder, "Latest attempt: `%s`\n\n", label)
		if events, ok := files[state.SchedulerEventsFileName]; ok {
			builder.WriteString("Scheduler query outcomes:\n\n")
			writeFence(builder, lastLines(string(events), 30))
		}
		if *probes > 0 && collector.probe(store, dirPrefix, dir) {
			*probes--
		}
		if showOutput {
			for _, name := range outputFiles {
				if data, ok := outputs[name]; ok {
					fmt.Fprintf(builder, "Last %d lines of %s:\n\n", summaryOutputLines, name)
					writeFence(builder, lastLines(string(data), summaryOutputLines))
				}
			}
		}
	}
}

func (collector *collector) jobOutputs(dirPrefix, dir string) map[string][]byte {
	outputs := make(map[string][]byte)
	if collector.options.Output == OutputNone {
		return outputs
	}
	limit := int64(outputTailBytes)
	if collector.options.Output == OutputFull {
		limit = outputFullBytes
	}
	for _, name := range outputFiles {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := readTail(path, limit)
		if err != nil {
			collector.note("%s not read: %v", dirPrefix+name, err)
			continue
		}
		if info.Size() > limit {
			collector.note("%s holds the last %d of %d bytes", dirPrefix+name, limit, info.Size())
		}
		collector.addFile(dirPrefix+name, data)
		outputs[name] = data
	}
	return outputs
}

// probe runs the scheduler's queries for the job in dir and reports whether
// it is a scheduler job.
func (collector *collector) probe(store state.Store, dirPrefix, dir string) bool {
	probes := executor.ProbeSchedulerJob(store, dir)
	if len(probes) == 0 {
		return false
	}
	var text strings.Builder
	for _, probe := range probes {
		fmt.Fprintf(&text, "$ %s\n%s", strings.Join(probe.Command, " "), probe.Output)
		if !strings.HasSuffix(probe.Output, "\n") && probe.Output != "" {
			text.WriteString("\n")
		}
		if probe.Error != "" {
			fmt.Fprintf(&text, "(error: %s)\n", probe.Error)
		}
	}
	collector.addFile(dirPrefix+"scheduler_probe.txt", []byte(text.String()))
	collector.summary.WriteString("Scheduler queries run now:\n\n")
	writeFence(&collector.summary, text.String())
	return true
}

// addStateFile adds a state file with secrets removed and returns what it
// added. A missing file is skipped; a JSON file that cannot be decoded is
// left out, since it cannot be checked for secrets.
func (collector *collector) addStateFile(name, path string) ([]byte, bool) {
	data, err := readLimited(path, stateFileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false
	}
	if err != nil {
		collector.note("%s not read: %v", name, err)
		return nil, false
	}
	switch {
	case strings.HasSuffix(name, ".jsonl"):
		data = redactJSONLines(data)
	case strings.HasSuffix(name, ".json") || strings.HasPrefix(strings.TrimSpace(string(data)), "{"):
		redactedData, err := redactJSON(data)
		if err != nil {
			collector.note("%s left out: not valid JSON (%v)", name, err)
			return nil, false
		}
		data = redactedData
	}
	collector.addFile(name, data)
	return data, true
}

func tableCell(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "|", "\\|")
	if len(value) > 120 {
		value = value[:117] + "..."
	}
	return value
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
