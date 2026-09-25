package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

const (
	reportLogLines          = 100
	reportLogChars          = 12000
	redactedPathPlaceholder = "[REDACTED_PATH]"
)

var (
	reportUnixPathPattern    = regexp.MustCompile(`(?:/home|/data|/tmp|/work|/mnt|/scratch|/opt|/var)/(?:[A-Za-z0-9._~-]+/)*[A-Za-z0-9._~-]*`)
	reportWindowsPathPattern = regexp.MustCompile(`[A-Za-z]:\\(?:[^\\\r\n ]+\\)*[^\\\r\n ]+`)
	reportFQDNPattern        = regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9.-]*\.(?:com|org|net|edu|gov|io|jp|local)\b`)
)

// buildAIReport formats run or job evidence for AI-assisted diagnosis.
func buildAIReport(paths state.ProjectPaths, runID, jobID string, failedOnly bool, attemptIDs ...string) (string, error) {
	run, err := loadAIReportRun(paths, runID, attemptIDs...)
	if err != nil {
		return "", err
	}
	if jobID != "" {
		if !state.IsValidPathElement(jobID) {
			return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
		}
		for _, job := range run.Jobs {
			if job.ID == jobID {
				return redactAIReport(formatJobAIReport(paths, run, job), paths, run), nil
			}
		}
		return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
	}
	return redactAIReport(formatRunAIReport(paths, run, failedOnly), paths, run), nil
}

// buildAIReportForJobs formats evidence for a selected set of jobs in one run.
func buildAIReportForJobs(paths state.ProjectPaths, runID string, jobIDs []string) (string, error) {
	run, err := loadAIReportRun(paths, runID)
	if err != nil {
		return "", err
	}
	selected := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		if !state.IsValidPathElement(jobID) {
			return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
		}
		selected[jobID] = true
	}
	for jobID := range selected {
		found := false
		for _, job := range run.Jobs {
			if job.ID == jobID {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
		}
	}
	return redactAIReport(formatRunAIReportSelected(paths, run, selected, false), paths, run), nil
}

func loadAIReportRun(paths state.ProjectPaths, runID string, attemptIDs ...string) (webprojection.Run, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return webprojection.Run{}, fmt.Errorf(runNotFoundMessage, runID)
	}
	var summary model.RunSummary
	if path, err := state.ValidatedStateFile(runDir, stateFileSummaryJSON); err == nil {
		summary, err = state.LoadRunSummary(path)
		if err != nil && !os.IsNotExist(err) {
			return webprojection.Run{}, fmt.Errorf("failed to read summary: %w", err)
		}
	} else {
		summary, err = state.LoadRunSummary(filepath.Join(runDir, stateFileSummaryJSON))
		if err != nil && !os.IsNotExist(err) {
			return webprojection.Run{}, fmt.Errorf("failed to read summary: %w", err)
		}
	}
	if summary.RunID == "" {
		summary.RunID = runID
	}
	running := runIsActive(paths, runID)
	if summary.Status == "" {
		if running {
			summary.Status = "running"
		} else {
			summary.Status = "unknown"
		}
	}
	jobs, err := loadWebJobs(runDir, summary, attemptIDs...)
	if err != nil {
		return webprojection.Run{}, err
	}
	context := model.RunContext{}
	if loaded, err := state.LoadContext(jsonStore(), runDir); err == nil {
		context = model.RunContext(loaded)
	}
	return webprojection.Run{RunSummary: summary, Jobs: jobs, CWD: context.CWD, Context: context, Running: running}, nil
}

func formatRunAIReport(paths state.ProjectPaths, run webprojection.Run, failedOnly bool) string {
	return formatRunAIReportSelected(paths, run, nil, failedOnly)
}

func formatRunAIReportSelected(paths state.ProjectPaths, run webprojection.Run, selected map[string]bool, failedOnly bool) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# rotari run report")
	fmt.Fprintf(&builder, "\n- Project: %s\n- Run ID: `%s`\n- Status: %s\n- Exit code: %d\n", paths.ProjectName, run.RunID, run.Status, run.ExitCode)
	fmt.Fprintf(&builder, "- Started: %s\n- Finished: %s\n- Host: %s\n- Working directory: `%s`\n", reportValue(model.FormatDisplayTimestamp(run.StartedAt)), reportValue(model.FormatDisplayTimestamp(run.FinishedAt)), reportValue(run.Context.Hostname), reportValue(run.CWD))
	for _, job := range run.Jobs {
		if selected != nil && !selected[job.ID] {
			continue
		}
		status := reportJobStatus(job, run.Running)
		if failedOnly && status != "failed" && status != "blocked" {
			continue
		}
		writeJobAIReport(&builder, paths, run, job, status, status == "failed" || status == "running" || status == "suspended")
	}
	fmt.Fprintf(&builder, "\n## Suggested commands\n```sh\nrotari show -r %s --failed-logs\nrotari retry -r %s\n```\n", executor.ShellQuote(run.RunID), executor.ShellQuote(run.RunID))
	return builder.String()
}

func formatJobAIReport(paths state.ProjectPaths, run webprojection.Run, job webprojection.Job) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# rotari job report")
	fmt.Fprintf(&builder, "\n- Project: %s\n- Run ID: `%s`\n- Run status: %s\n- Host: %s\n", paths.ProjectName, run.RunID, run.Status, reportValue(run.Context.Hostname))
	writeJobAIReport(&builder, paths, run, job, reportJobStatus(job, run.Running), true)
	return builder.String()
}

func redactAIReport(report string, paths state.ProjectPaths, run webprojection.Run) string {
	values := []string{paths.BaseDir, run.CWD, run.Context.Hostname}
	for _, job := range run.Jobs {
		values = append(values, job.WorkingDirectory)
		if job.Origin != nil {
			values = append(values, job.Origin.CWD)
		}
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	values = values[:0]
	for value := range unique {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	replacements := make([]string, 0, len(values)*2)
	for _, value := range values {
		replacement := redactedPathPlaceholder
		if value == run.Context.Hostname {
			replacement = "[REDACTED_HOST]"
		}
		replacements = append(replacements, value, replacement)
	}
	if len(replacements) > 0 {
		report = strings.NewReplacer(replacements...).Replace(report)
	}
	report = reportUnixPathPattern.ReplaceAllString(report, redactedPathPlaceholder)
	report = reportWindowsPathPattern.ReplaceAllString(report, redactedPathPlaceholder)
	report = reportFQDNPattern.ReplaceAllString(report, "[REDACTED_HOST]")
	return report + "\n> Paths and hostnames are redacted where detected. Review logs before sharing; complete redaction is not guaranteed.\n"
}

func writeJobAIReport(builder *strings.Builder, paths state.ProjectPaths, run webprojection.Run, job webprojection.Job, status string, includeLog bool) {
	name := job.Name
	if name == "" {
		name = job.ID
	}
	executor := job.Executor
	if executor == "" {
		executor = "default"
	}
	result := job.Result
	fmt.Fprintf(builder, "\n## Job: %s\n- Job ID: `%s`\n- Status: %s\n- Executor: %s\n", name, job.ID, status, executor)
	if job.AttemptID != "" {
		fmt.Fprintf(builder, "- Attempt ID: `%s`\n", job.AttemptID)
	}
	fmt.Fprintf(builder, "- Dependencies: %s\n- Working directory: `%s`\n- Started: %s\n- Finished: %s\n", reportValue(model.FormatDependencies(job.DependsOn, job.DependsOnFinished, ", ")), reportValue(firstNonEmpty(job.WorkingDirectory, run.CWD)), reportValue(model.FormatDisplayTimestamp(job.SubmittedAt)), reportValue(model.FormatDisplayTimestamp(job.FinishedAt)))
	if result == nil {
		fmt.Fprintln(builder, "- Exit code: -")
	} else {
		fmt.Fprintf(builder, "- Exit code: %d\n", result.ExitCode)
		if result.Error != "" {
			fmt.Fprintf(builder, "- Error: %s\n", result.Error)
		}
	}
	command, _ := json.Marshal(job.Command)
	fmt.Fprintf(builder, "\n### Command\n```json\n%s\n```\n", command)
	if result != nil {
		writeReportDiagnoses(builder, *result)
	}
	if includeLog {
		if output := readReportLog(paths, run.RunID, job); output != "" {
			fmt.Fprintf(builder, "\n### Log (last %d lines, at most %d characters)\n```text\n%s\n```\n", reportLogLines, reportLogChars, output)
		}
	}
}

func reportJobStatus(job webprojection.Job, running bool) string {
	if job.Result == nil {
		if job.SchedulerState != "" {
			return job.SchedulerState
		}
		if running {
			return "running"
		}
		return "pending"
	}
	if strings.HasPrefix(job.Result.Error, "blocked") {
		return "blocked"
	}
	if job.Result.ExitCode == 0 {
		return "success"
	}
	return "failed"
}

func readReportLog(paths state.ProjectPaths, runID string, job webprojection.Job) string {
	if job.AttemptDir != "" {
		// NOSONAR: job.AttemptDir is created from validated path elements only.
		path, err := state.ValidatedStateFile(job.AttemptDir, stateFileOutput)
		if err != nil {
			return ""
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		return tailReportLog(string(data))
	}
	jobID := job.ID
	if job.Origin != nil {
		runID = job.Origin.RunID
		jobID = job.Origin.JobID
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return ""
	}
	jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
	if err != nil {
		return ""
	}
	path, err := state.ValidatedStateFile(jobDir, "output")
	if err != nil {
		return ""
	}
	// codeql[go/path-injection]: path is restricted by validatedStateFile to output.
	data, err := os.ReadFile(path) // NOSONAR: path is restricted by validatedStateFile to output.
	if err != nil {
		return ""
	}
	return tailReportLog(string(data))
}

func tailReportLog(data string) string {
	lines := strings.Split(data, "\n")
	if len(lines) > reportLogLines {
		lines = lines[len(lines)-reportLogLines:]
	}
	output := []rune(strings.Join(lines, "\n"))
	if len(output) > reportLogChars {
		output = output[len(output)-reportLogChars:]
	}
	return string(output)
}

func reportValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeReportDiagnoses(builder *strings.Builder, result model.JobResult) {
	switch {
	case result.DiagnosisStatus == model.DiagnosisNoMatch:
		fmt.Fprintf(builder, "\n### Diagnosis\n- No known rule matched.\n  Next: %s\n", noMatchDiagnosisNext)
	case result.DiagnosisStatus == model.DiagnosisUnavailable:
		fmt.Fprintf(builder, "\n### Diagnosis\n- Unavailable: %s\n  Next: %s\n", result.DiagnosisNote, unavailableDiagnosisNext)
	case len(result.Diagnoses) > 0:
		fmt.Fprintln(builder, "\n### Diagnosis")
		for _, diagnosis := range result.Diagnoses {
			fmt.Fprintf(builder, "- %s\n  Evidence: %s\n  Next: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
		}
	default:
		return
	}
	if diagnose.Outdated(result) {
		fmt.Fprintf(builder, "\nNote: %s\n", outdatedDiagnosisNote)
	}
}
