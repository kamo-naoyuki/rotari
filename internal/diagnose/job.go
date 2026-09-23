package diagnose

import (
	"fmt"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type Job struct {
	RunID    string
	JobID    string
	Command  []string
	ExitCode *int
	Error    string
	Log      string
}

func DiagnoseDefault(job Job) []model.RuleDiagnosis {
	return Diagnose(job.Error, job.Log, DefaultRules())
}

func BuildPrompt(job Job, language string) string {
	status := "not recorded"
	if job.ExitCode != nil {
		status = fmt.Sprintf("exit code %d", *job.ExitCode)
	}
	languageInstruction := ""
	if language != "" {
		languageInstruction = fmt.Sprintf(" Respond in the language identified by the BCP 47 tag %q.", language)
	}
	return fmt.Sprintf("Diagnose this failed rotari job. Explain the likely root cause, cite evidence from the log, and give minimal concrete next steps. Do not claim to have executed anything.%s\n\nRun: %s\nJob: %s\nStatus: %s\nScheduler error: %s\nCommand: %s\n\nLog tail:\n%s", languageInstruction, job.RunID, job.JobID, status, job.Error, strings.Join(job.Command, " "), job.Log)
}

func IsLanguageTag(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, part := range parts {
		if !isLanguagePart(part) {
			return false
		}
	}
	return true
}

func isLanguagePart(part string) bool {
	if len(part) == 0 || len(part) > 8 {
		return false
	}
	for _, character := range part {
		if !('a' <= character && character <= 'z' || 'A' <= character && character <= 'Z' || '0' <= character && character <= '9') {
			return false
		}
	}
	return true
}

func TailLog(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return "[earlier log output omitted]\n" + value[len(value)-limit:]
}
