package diagnose

import (
	"regexp"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type Rule struct {
	Name       string
	Patterns   []*regexp.Regexp
	Excludes   []*regexp.Regexp
	Suggestion string
}

var (
	ansiEscapeSequence  = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	pythonExceptionRule = regexp.MustCompile(`^(?:[a-z_][a-z0-9_.]*\.)?[a-z_][a-z0-9_]*(?:error|exception): .+`)
)

func Diagnose(errorText, log string, rules []Rule) []model.RuleDiagnosis {
	lines := strings.Split(errorText+"\n"+log, "\n")
	var diagnoses []model.RuleDiagnosis
	for _, rule := range rules {
		for _, line := range lines {
			evidence := strings.TrimSpace(ansiEscapeSequence.ReplaceAllString(line, ""))
			normalized := strings.Join(strings.Fields(strings.ToLower(evidence)), " ")
			if MatchesAny(normalized, rule.Excludes) {
				continue
			}
			for _, pattern := range rule.Patterns {
				if pattern.MatchString(normalized) {
					diagnoses = append(diagnoses, model.RuleDiagnosis{Name: rule.Name, Evidence: evidence, Suggestion: rule.Suggestion})
					goto nextRule
				}
			}
		}
	nextRule:
	}
	if exception := findPythonException(lines); exception != "" {
		diagnoses = append(diagnoses, model.RuleDiagnosis{Name: "Python exception", Evidence: exception, Suggestion: "Inspect the traceback and the failing call named above; fix the reported exception before retrying."})
	}
	return diagnoses
}

func MatchesAny(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func findPythonException(lines []string) string {
	hasTraceback := false
	exception := ""
	for _, line := range lines {
		evidence := strings.TrimSpace(ansiEscapeSequence.ReplaceAllString(line, ""))
		if strings.EqualFold(evidence, "Traceback (most recent call last):") {
			hasTraceback = true
			continue
		}
		if hasTraceback && pythonExceptionRule.MatchString(strings.ToLower(evidence)) {
			exception = evidence
		}
	}
	return exception
}
