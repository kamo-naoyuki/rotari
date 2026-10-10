package report

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

// reportTableLineRunes limits a log line quoted in the job table.
const reportTableLineRunes = 160

// writeReportSources lists the revision each source repository was at, as
// show does.
func writeReportSources(builder *strings.Builder, sources []model.SourceRevision) {
	for _, source := range sources {
		fmt.Fprintf(builder, "- Source: `%s`\n", model.SourceLabel(source))
	}
}

// writeReportNotes writes notes under heading, oldest first, each as its
// time and its text as written, which may be Markdown. A note on an attempt
// other than currentAttemptID names that attempt.
func writeReportNotes(builder *strings.Builder, heading string, notes []model.RunNote, currentAttemptID string) {
	if len(notes) == 0 {
		return
	}
	fmt.Fprintf(builder, "\n%s Notes\n", heading)
	for _, note := range notes {
		label := model.FormatDisplayTimestamp(note.At)
		if note.AttemptID != "" && note.AttemptID != currentAttemptID {
			label += " (attempt `" + note.AttemptID + "`)"
		}
		fmt.Fprintf(builder, "\n**%s**\n\n%s\n", label, strings.TrimSpace(note.Text))
	}
}

// writeReportJobTable writes one row per job: its name, the environment
// values that tell the jobs apart (matrix values among them), its status
// and exit code, and the first and last line of its log, which often hold
// the configuration a program started with and its final metric or error.
func writeReportJobTable(builder *strings.Builder, paths state.ProjectPaths, run webprojection.Run, jobs []reportJob) {
	if len(jobs) == 0 {
		return
	}
	variables := distinguishingVariables(jobs)
	header := []string{"Job"}
	hasTask := false
	for _, job := range jobs {
		hasTask = hasTask || job.job.ArrayTaskID != nil
	}
	if hasTask {
		header = append(header, "Task")
	}
	header = append(header, variables...)
	header = append(header, "Status", "Exit", "First log line", "Last log line")
	fmt.Fprintln(builder, "\n## Jobs")
	fmt.Fprintf(builder, "\n| %s |\n|%s\n", strings.Join(header, " | "), strings.Repeat(" --- |", len(header)))
	for _, entry := range jobs {
		job := entry.job
		row := []string{reportTableText(reportJobName(job))}
		if hasTask {
			task := "-"
			if job.ArrayTaskID != nil {
				task = strconv.Itoa(*job.ArrayTaskID)
			}
			row = append(row, task)
		}
		values := environmentValues(job.Environment)
		for _, name := range variables {
			value, ok := values[name]
			if !ok {
				row = append(row, "-")
				continue
			}
			row = append(row, reportTableCode(value))
		}
		exit := "-"
		if job.Result != nil {
			exit = strconv.Itoa(job.Result.ExitCode)
		}
		first, last := reportLogEnds(readReportLog(paths, run.RunID, job))
		row = append(row, reportTableText(entry.status), exit, reportTableCode(first), reportTableCode(last))
		fmt.Fprintf(builder, "| %s |\n", strings.Join(row, " | "))
	}
}

// reportJob is a job a run report includes, with its status label.
type reportJob struct {
	job    webprojection.Job
	status string
}

func reportJobName(job webprojection.Job) string {
	if job.Name != "" {
		return job.Name
	}
	return job.ID
}

// distinguishingVariables names, in the order they first appear, the
// environment variables whose value is not the same for every job, so the
// table shows what tells the jobs apart and not what they share.
func distinguishingVariables(jobs []reportJob) []string {
	var names []string
	seen := map[string]bool{}
	for _, entry := range jobs {
		for _, assignment := range entry.job.Environment {
			name, _, _ := strings.Cut(assignment, "=")
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	varying := names[:0]
	for _, name := range names {
		first, firstSet := environmentValues(jobs[0].job.Environment)[name]
		for _, entry := range jobs[1:] {
			value, set := environmentValues(entry.job.Environment)[name]
			if set != firstSet || value != first {
				varying = append(varying, name)
				break
			}
		}
	}
	return varying
}

func environmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, assignment := range environment {
		name, value, _ := strings.Cut(assignment, "=")
		values[name] = value
	}
	return values
}

// reportLogEnds returns the first and last non-blank lines of a job's log,
// skipping the stream headers readSeparateJobLogs adds.
func reportLogEnds(log string) (string, string) {
	var first, last string
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || line == "--- "+state.StdoutFileName+" ---" || line == "--- "+state.StderrFileName+" ---" {
			continue
		}
		if first == "" {
			first = line
		}
		last = line
	}
	return first, last
}

// reportTableText escapes text for a Markdown table cell.
func reportTableText(text string) string {
	if text == "" {
		return "-"
	}
	return strings.ReplaceAll(text, "|", `\|`)
}

// reportTableCode quotes text, shortened to reportTableLineRunes, as code in
// a Markdown table cell, so that log text is not read as Markdown.
func reportTableCode(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "-"
	}
	if utf8.RuneCountInString(text) > reportTableLineRunes {
		text = string([]rune(text)[:reportTableLineRunes-1]) + "…"
	}
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}
	return strings.ReplaceAll(fence+text+fence, "|", `\|`)
}
