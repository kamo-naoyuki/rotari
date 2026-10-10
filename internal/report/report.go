package report

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
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

const (
	jobNotFoundMessage = "job %q not found in run %q"
	runNotFoundMessage = "run %q not found"
	reportLogLines     = 100
	reportLogChars     = 12000
	// When the log contains diagnosis evidence, the excerpt keeps these
	// lines around the latest line with each evidence, and this many final
	// lines, instead of the last reportLogLines lines.
	reportEvidenceBefore    = 20
	reportEvidenceAfter     = 5
	reportEvidenceTail      = 20
	redactedPathPlaceholder = "[REDACTED_PATH]"
)

var (
	reportUnixPathPattern    = regexp.MustCompile(`(?:/home|/data|/tmp|/work|/mnt|/scratch|/opt|/var)/(?:[A-Za-z0-9._~-]+/)*[A-Za-z0-9._~-]*`)
	reportWindowsPathPattern = regexp.MustCompile(`[A-Za-z]:\\(?:[^\\\r\n ]+\\)*[^\\\r\n ]+`)
	reportFQDNPattern        = regexp.MustCompile(`\b[A-Za-z0-9][A-Za-z0-9.-]*\.(?:com|org|net|edu|gov|io|jp|local)\b`)
)

// Build formats the evidence of runID, or of its job jobID, for AI-assisted
// diagnosis. failedOnly limits a run report to failed and blocked jobs. A
// non-empty attemptID shows that attempt instead of its job's latest. redact
// replaces paths and hostnames with placeholders; disable it only for
// sharing within a trusted team.
func Build(store state.Store, paths state.ProjectPaths, runID, jobID string, failedOnly bool, attemptID string, redact bool) (string, error) {
	run, err := loadRun(store, paths, runID, attemptID)
	if err != nil {
		return "", err
	}
	if jobID != "" {
		if !state.IsValidPathElement(jobID) {
			return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
		}
		for _, job := range run.Jobs {
			if job.ID == jobID {
				return finishAIReport(formatJobAIReport(paths, run, job), paths, run, redact), nil
			}
		}
		return "", fmt.Errorf(jobNotFoundMessage, jobID, runID)
	}
	return finishAIReport(formatRunAIReport(paths, run, failedOnly), paths, run, redact), nil
}

// BuildForJobs formats the evidence of the selected jobs of runID as a run
// report. See Build for redact.
func BuildForJobs(store state.Store, paths state.ProjectPaths, runID string, jobIDs []string, redact bool) (string, error) {
	run, err := loadRun(store, paths, runID, "")
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
	return finishAIReport(formatRunAIReportSelected(paths, run, selected, false), paths, run, redact), nil
}

func loadRun(store state.Store, paths state.ProjectPaths, runID, attemptID string) (webprojection.Run, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return webprojection.Run{}, fmt.Errorf(runNotFoundMessage, runID)
	}
	var summary model.RunSummary
	if path, err := state.ValidatedStateFile(runDir, "summary.json"); err == nil {
		summary, err = state.LoadRunSummary(path)
		if err != nil && !os.IsNotExist(err) {
			return webprojection.Run{}, fmt.Errorf("failed to read summary: %w", err)
		}
	} else {
		summary, err = state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
		if err != nil && !os.IsNotExist(err) {
			return webprojection.Run{}, fmt.Errorf("failed to read summary: %w", err)
		}
	}
	if summary.RunID == "" {
		summary.RunID = runID
	}
	running := project.RunActive(paths, runID)
	if summary.Status == "" {
		if running {
			summary.Status = "running"
		} else {
			summary.Status = "unknown"
		}
	}
	jobs, err := webprojection.LoadRunJobs(store, runDir, summary, attemptID)
	if err != nil {
		return webprojection.Run{}, err
	}
	context := model.RunContext{}
	if loaded, err := state.LoadContext(store, runDir); err == nil {
		context = model.RunContext(loaded)
	}
	run := webprojection.Run{RunSummary: summary, Jobs: jobs, CWD: context.CWD, Context: context, Running: running}
	if sources, ok, err := state.LoadRunSources(runDir); err == nil && ok {
		run.Sources = sources.Sources
	}
	if notes, err := state.LoadRunNotes(runDir); err == nil {
		run.Notes = notes
	}
	return run, nil
}

func formatRunAIReport(paths state.ProjectPaths, run webprojection.Run, failedOnly bool) string {
	return formatRunAIReportSelected(paths, run, nil, failedOnly)
}

func formatRunAIReportSelected(paths state.ProjectPaths, run webprojection.Run, selected map[string]bool, failedOnly bool) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# rotari run report")
	fmt.Fprintf(&builder, "\n- Project: %s\n- Run ID: `%s`\n- Status: %s\n- Exit code: %d\n", paths.ProjectName, run.RunID, run.Status, run.ExitCode)
	fmt.Fprintf(&builder, "- Started: %s\n- Finished: %s\n- Host: %s\n- Working directory: `%s`\n", reportValue(model.FormatDisplayTimestamp(run.StartedAt)), reportValue(model.FormatDisplayTimestamp(run.FinishedAt)), reportValue(run.Context.Hostname), reportValue(run.CWD))
	writeReportSources(&builder, run.Sources)
	writeReportNotes(&builder, "##", model.RunNotesFor(run.Notes, ""), "")
	var jobs []reportJob
	for _, job := range run.Jobs {
		if selected != nil && !selected[job.ID] {
			continue
		}
		status := reportJobStatus(job)
		if failedOnly && !reportFailed(status) {
			continue
		}
		jobs = append(jobs, reportJob{job: job, status: status})
	}
	writeReportJobTable(&builder, paths, run, jobs)
	for _, entry := range jobs {
		writeJobAIReport(&builder, paths, run, entry.job, entry.status, reportIncludesLog(entry.status))
	}
	fmt.Fprintf(&builder, "\n## Suggested commands\n```sh\nrotari show -r %s --failed-logs\nrotari retry -r %s\n```\n", executor.ShellQuote(run.RunID), executor.ShellQuote(run.RunID))
	return builder.String()
}

func formatJobAIReport(paths state.ProjectPaths, run webprojection.Run, job webprojection.Job) string {
	var builder strings.Builder
	fmt.Fprintln(&builder, "# rotari job report")
	fmt.Fprintf(&builder, "\n- Project: %s\n- Run ID: `%s`\n- Run status: %s\n- Host: %s\n", paths.ProjectName, run.RunID, run.Status, reportValue(run.Context.Hostname))
	writeReportSources(&builder, run.Sources)
	writeJobAIReport(&builder, paths, run, job, reportJobStatus(job), true)
	return builder.String()
}

// finishAIReport optionally redacts report, then appends the notice matching
// that choice.
func finishAIReport(report string, paths state.ProjectPaths, run webprojection.Run, redact bool) string {
	if !redact {
		return report + "\n> Redaction is turned off for this report. Review paths and hostnames before sharing.\n"
	}
	return redactAIReport(report, paths, run)
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
	return RedactPatterns(report) + "\n> Paths and hostnames are redacted where detected. Review logs before sharing; complete redaction is not guaranteed.\n"
}

// RedactPatterns replaces text that looks like an absolute path or a
// hostname, the generic part of a report's redaction. It does not know the
// run's own paths and hostname, so it can miss some; see Build.
func RedactPatterns(text string) string {
	text = reportUnixPathPattern.ReplaceAllString(text, redactedPathPlaceholder)
	text = reportWindowsPathPattern.ReplaceAllString(text, redactedPathPlaceholder)
	return reportFQDNPattern.ReplaceAllString(text, "[REDACTED_HOST]")
}

func writeJobAIReport(builder *strings.Builder, paths state.ProjectPaths, run webprojection.Run, job webprojection.Job, status string, includeLog bool) {
	name := reportJobName(job)
	executor := state.ReadAttemptExecutor(job.AttemptDir)
	if executor == "" {
		executor = job.Executor
	}
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
	writeReportNotes(builder, "###", model.RunNotesFor(run.Notes, job.ID), job.AttemptID)
	command, _ := json.Marshal(job.Command)
	fmt.Fprintf(builder, "\n### Command\n```json\n%s\n```\n", command)
	if result != nil {
		writeReportDiagnoses(builder, *result)
	}
	if includeLog {
		var evidence []string
		if result != nil {
			for _, diagnosis := range result.Diagnoses {
				evidence = append(evidence, diagnosis.Evidence)
			}
		}
		if output, description := reportLogExcerpt(readReportLog(paths, run.RunID, job), evidence); output != "" {
			fmt.Fprintf(builder, "\n### Log (%s)\n```text\n%s\n```\n", description, output)
		}
	}
}

// reportJobStatus labels job as show does, from the shared execution status.
func reportJobStatus(job webprojection.Job) string {
	status := strings.TrimSuffix(job.ExecutionStatus, " (carried)")
	if status == "" {
		status = "unknown"
	}
	return jobstatus.DisplayLabel(status, job.Result != nil && job.Result.Accepted, job.Carried)
}

// reportFailed reports whether a run report limited to failures includes a
// job with this label: failed, cancelled, and blocked jobs, carried or not.
func reportFailed(label string) bool {
	switch strings.TrimSuffix(label, " (carried)") {
	case model.StatusFailed, model.StatusCancelled, "blocked":
		return true
	}
	return false
}

// reportIncludesLog reports whether a run report quotes the log of a job
// with this label: one that failed or may not have finished.
func reportIncludesLog(label string) bool {
	switch strings.TrimSuffix(label, " (carried)") {
	case model.StatusFailed, model.StatusCancelled, "running (recorded)", "suspended (recorded)", "unknown":
		return true
	}
	return false
}

func readReportLog(paths state.ProjectPaths, runID string, job webprojection.Job) string {
	if job.AttemptDir != "" {
		return readSeparateJobLogs(job.AttemptDir)
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
	return readSeparateJobLogs(jobDir)
}

func readSeparateJobLogs(jobDir string) string {
	if data, err := os.ReadFile(filepath.Join(jobDir, "output")); err == nil {
		return string(data)
	}
	var logs strings.Builder
	for _, stream := range []string{state.StdoutFileName, state.StderrFileName} {
		path, err := state.ValidatedStateFile(jobDir, stream)
		if err != nil {
			continue
		}
		// codeql[go/path-injection]: path is restricted by ValidatedStateFile to stdout or stderr.
		data, err := os.ReadFile(path) // NOSONAR: path is restricted to the two validated stream filenames.
		if err != nil || len(data) == 0 {
			continue
		}
		if logs.Len() > 0 {
			logs.WriteString("\n")
		}
		fmt.Fprintf(&logs, "--- %s ---\n", stream)
		logs.Write(data)
	}
	return logs.String()
}

// reportLogExcerpt selects the log lines a report shows and describes the
// selection. When the log contains diagnosis evidence, it keeps the lines
// around the latest line containing each evidence and the final lines,
// marking the lines it omits, so the cause is shown even when it is far from
// the end. Otherwise it keeps the last reportLogLines lines. Either way it
// keeps at most reportLogChars characters, from the end.
func reportLogExcerpt(data string, evidence []string) (string, string) {
	lines := strings.Split(data, "\n")
	keep := make([]bool, len(lines))
	found := false
	for _, text := range evidence {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		for index := len(lines) - 1; index >= 0; index-- {
			if strings.Contains(lines[index], text) {
				keepLines(keep, index-reportEvidenceBefore, index+reportEvidenceAfter)
				found = true
				break
			}
		}
	}
	if !found {
		if len(lines) > reportLogLines {
			lines = lines[len(lines)-reportLogLines:]
		}
		return limitReportLog(strings.Join(lines, "\n")), fmt.Sprintf("last %d lines, at most %d characters", reportLogLines, reportLogChars)
	}
	keepLines(keep, len(lines)-reportEvidenceTail, len(lines)-1)
	selected := make([]string, 0, len(lines))
	omitted := 0
	for index, line := range lines {
		if !keep[index] {
			omitted++
			continue
		}
		if omitted > 0 {
			selected = append(selected, fmt.Sprintf("[... %d lines omitted ...]", omitted))
			omitted = 0
		}
		selected = append(selected, line)
	}
	return limitReportLog(strings.Join(selected, "\n")), fmt.Sprintf("lines around the diagnosis evidence and the last %d lines, at most %d characters", reportEvidenceTail, reportLogChars)
}

// keepLines marks lines first through last, clamped to keep's bounds.
func keepLines(keep []bool, first, last int) {
	for index := max(first, 0); index <= last && index < len(keep); index++ {
		keep[index] = true
	}
}

// limitReportLog keeps the last reportLogChars characters of output.
func limitReportLog(output string) string {
	runes := []rune(output)
	if len(runes) > reportLogChars {
		runes = runes[len(runes)-reportLogChars:]
	}
	return string(runes)
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
		fmt.Fprintf(builder, "\n### Diagnosis\n- No known rule matched.\n  Next: %s\n", diagnose.NoMatchNext)
	case result.DiagnosisStatus == model.DiagnosisUnavailable:
		fmt.Fprintf(builder, "\n### Diagnosis\n- Unavailable: %s\n  Next: %s\n", result.DiagnosisNote, diagnose.UnavailableNext)
	case len(result.Diagnoses) > 0:
		fmt.Fprintln(builder, "\n### Diagnosis")
		for _, diagnosis := range result.Diagnoses {
			fmt.Fprintf(builder, "- %s\n  Evidence: %s\n  Next: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
		}
	default:
		return
	}
	if diagnose.Outdated(result) {
		fmt.Fprintf(builder, "\nNote: %s\n", diagnose.OutdatedNote)
	}
}
