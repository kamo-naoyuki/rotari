package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	reportLogLines = 100
	reportLogChars = 12000
)

var (
	reportUnixPathPattern    = regexp.MustCompile(`(?:/home|/data|/tmp|/work|/mnt|/scratch|/opt|/var)/(?:[A-Za-z0-9._~-]+/)*[A-Za-z0-9._~-]*`)
	reportWindowsPathPattern = regexp.MustCompile(`[A-Za-z]:\\(?:[^\\\r\n ]+\\)*[^\\\r\n ]+`)
	reportFQDNPattern        = regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9.-]*\.(?:com|org|net|edu|gov|io|jp|local)\b`)
)

func buildAIReport(paths pathSet, runID, jobID string, failedOnly bool) (string, error) {
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		return "", fmt.Errorf(runNotFoundMessage, runID)
	}
	summary, err := loadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read summary: %w", err)
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
	jobs, err := loadWebJobs(runDir, summary)
	if err != nil {
		return "", err
	}
	context := RunContext{}
	if path, err := validatedStateFile(runDir, "context.json"); err == nil {
		if data, readErr := os.ReadFile(path); readErr == nil {
			_ = json.Unmarshal(data, &context)
		}
	}
	run := webRun{RunSummary: summary, Jobs: jobs, CWD: context.CWD, Context: context, Running: running}
	if jobID != "" {
		if !isValidPathElement(jobID) {
			return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
		}
		for _, job := range jobs {
			if job.ID == jobID {
				return redactAIReport(formatJobAIReport(paths, run, job), paths, run), nil
			}
		}
		return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
	}
	return redactAIReport(formatRunAIReport(paths, run, failedOnly), paths, run), nil
}

func formatRunAIReport(paths pathSet, run webRun, failedOnly bool) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# rotari run report")
	fmt.Fprintf(&builder, "\n- Project: %s\n- Run ID: `%s`\n- Status: %s\n- Exit code: %d\n", paths.queueName, run.RunID, run.Status, run.ExitCode)
	fmt.Fprintf(&builder, "- Started: %s\n- Finished: %s\n- Host: %s\n- Working directory: `%s`\n", reportValue(formatDisplayTimestamp(run.StartedAt)), reportValue(formatDisplayTimestamp(run.FinishedAt)), reportValue(run.Context.Hostname), reportValue(run.CWD))
	for _, job := range run.Jobs {
		status := reportJobStatus(job, run.Running)
		if failedOnly && status != "failed" && status != "blocked" {
			continue
		}
		writeJobAIReport(&builder, paths, run, job, status, status == "failed" || status == "running" || status == "suspended")
	}
	fmt.Fprintf(&builder, "\n## Suggested commands\n```sh\nrotari show -r %s --failed-logs\nrotari retry -r %s\n```\n", shellQuote(run.RunID), shellQuote(run.RunID))
	return builder.String()
}

func formatJobAIReport(paths pathSet, run webRun, job webJob) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# rotari job report")
	fmt.Fprintf(&builder, "\n- Project: %s\n- Run ID: `%s`\n- Run status: %s\n- Host: %s\n", paths.queueName, run.RunID, run.Status, reportValue(run.Context.Hostname))
	writeJobAIReport(&builder, paths, run, job, reportJobStatus(job, run.Running), true)
	return builder.String()
}

func redactAIReport(report string, paths pathSet, run webRun) string {
	values := []string{paths.baseDir, run.CWD, run.Context.Hostname}
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
		replacement := "[REDACTED_PATH]"
		if value == run.Context.Hostname {
			replacement = "[REDACTED_HOST]"
		}
		replacements = append(replacements, value, replacement)
	}
	if len(replacements) > 0 {
		report = strings.NewReplacer(replacements...).Replace(report)
	}
	report = reportUnixPathPattern.ReplaceAllString(report, "[REDACTED_PATH]")
	report = reportWindowsPathPattern.ReplaceAllString(report, "[REDACTED_PATH]")
	report = reportFQDNPattern.ReplaceAllString(report, "[REDACTED_HOST]")
	return report + "\n> Paths and hostnames are redacted where detected. Review logs before sharing; complete redaction is not guaranteed.\n"
}

func writeJobAIReport(builder *strings.Builder, paths pathSet, run webRun, job webJob, status string, includeLog bool) {
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
	fmt.Fprintf(builder, "- Dependencies: %s\n- Working directory: `%s`\n- Started: %s\n- Finished: %s\n", reportValue(strings.Join(job.DependsOn, ", ")), reportValue(firstNonEmpty(job.WorkingDirectory, run.CWD)), reportValue(formatDisplayTimestamp(job.SubmittedAt)), reportValue(formatDisplayTimestamp(job.FinishedAt)))
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
	if result != nil && len(result.Diagnoses) > 0 {
		fmt.Fprintln(builder, "\n### Diagnosis")
		for _, diagnosis := range result.Diagnoses {
			fmt.Fprintf(builder, "- %s\n  Evidence: %s\n  Next: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
		}
	}
	if includeLog {
		if output := readReportLog(paths, run.RunID, job); output != "" {
			fmt.Fprintf(builder, "\n### Log (last %d lines, at most %d characters)\n```text\n%s\n```\n", reportLogLines, reportLogChars, output)
		}
	}
}

func reportJobStatus(job webJob, running bool) string {
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

func readReportLog(paths pathSet, runID string, job webJob) string {
	jobID := job.ID
	if job.Origin != nil {
		runID = job.Origin.RunID
		jobID = job.Origin.JobID
	}
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		return ""
	}
	jobDir, err := validatedJobDir(runDir, jobID)
	if err != nil {
		return ""
	}
	path, err := validatedStateFile(jobDir, "output")
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
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
