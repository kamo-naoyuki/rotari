package diagnose

import (
	"regexp"
	"sort"
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

type ruleMatch struct {
	line      int
	diagnosis model.RuleDiagnosis
}

// Diagnose returns one diagnosis per matching rule, citing the rule's latest
// matching line. Diagnoses are ordered from the latest evidence to the
// earliest, because the error that ended a job is normally reported last and
// earlier matches may be warnings the job recovered from. The scheduler error
// is recorded after the job output, so it ranks as the latest evidence.
func Diagnose(errorText, log string, rules []Rule) []model.RuleDiagnosis {
	lines := append(strings.Split(log, "\n"), strings.Split(errorText, "\n")...)
	evidence := make([]string, len(lines))
	normalized := make([]string, len(lines))
	for index, line := range lines {
		evidence[index] = strings.TrimSpace(ansiEscapeSequence.ReplaceAllString(line, ""))
		normalized[index] = strings.Join(strings.Fields(strings.ToLower(evidence[index])), " ")
	}

	var matches []ruleMatch
	explained := map[int]bool{}
	for _, rule := range rules {
		if line := lastMatchingLine(normalized, rule); line >= 0 {
			matches = append(matches, ruleMatch{line: line, diagnosis: model.RuleDiagnosis{Name: rule.Name, Evidence: evidence[line], Suggestion: rule.Suggestion}})
			explained[line] = true
		}
	}
	// The generic Python exception is reported only when no specific rule
	// already explains the same final exception line.
	if line := findPythonException(evidence); line >= 0 && !explained[line] {
		matches = append(matches, ruleMatch{line: line, diagnosis: model.RuleDiagnosis{Name: "Python exception", Evidence: evidence[line], Suggestion: "Inspect the traceback and the failing call named above; fix the reported exception before retrying."}})
	}

	sort.SliceStable(matches, func(i, j int) bool { return matches[i].line > matches[j].line })
	var diagnoses []model.RuleDiagnosis
	for _, match := range matches {
		diagnoses = append(diagnoses, match.diagnosis)
	}
	return diagnoses
}

func lastMatchingLine(normalized []string, rule Rule) int {
	for index := len(normalized) - 1; index >= 0; index-- {
		if !MatchesAny(normalized[index], rule.Excludes) && MatchesAny(normalized[index], rule.Patterns) {
			return index
		}
	}
	return -1
}

func MatchesAny(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func findPythonException(evidence []string) int {
	hasTraceback := false
	exception := -1
	for index, line := range evidence {
		if strings.EqualFold(line, "Traceback (most recent call last):") {
			hasTraceback = true
			continue
		}
		if hasTraceback && pythonExceptionRule.MatchString(strings.ToLower(line)) {
			exception = index
		}
	}
	return exception
}
