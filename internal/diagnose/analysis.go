package diagnose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Guidance shown with a saved rule diagnosis by show and reports.
const (
	// NoMatchNext follows a failure that no rule matched.
	NoMatchNext = "Inspect the full job output and scheduler accounting for the failure details."
	// UnavailableNext follows a diagnosis that could not read the output.
	UnavailableNext = "Resolve the read error, then run rotari diagnose --rules."
	// OutdatedNote marks a diagnosis saved under earlier rules; see Outdated.
	OutdatedNote = "Saved with earlier diagnosis rules; rotari diagnose --rules shows the result under the current rules."
)

// matcherRevision is part of RulesVersion. Bump it when Diagnose changes how
// rules match or rank lines without changing any rule definition.
const matcherRevision = 1

var rulesVersion = sync.OnceValue(computeRulesVersion)

func computeRulesVersion() string {
	hash := sha256.New()
	fmt.Fprintf(hash, "matcher %d\npython %s\n", matcherRevision, pythonExceptionRule)
	for _, rule := range defaultRules {
		fmt.Fprintf(hash, "rule %q %q\n", rule.Name, rule.Suggestion)
		for _, pattern := range rule.Patterns {
			fmt.Fprintf(hash, "pattern %q\n", pattern)
		}
		for _, exclude := range rule.Excludes {
			fmt.Fprintf(hash, "exclude %q\n", exclude)
		}
	}
	return hex.EncodeToString(hash.Sum(nil))[:12]
}

// RulesVersion identifies the default rules and matcher. It is derived from
// the rule definitions, so any rule change produces a new version.
func RulesVersion() string {
	return rulesVersion()
}

// AnalyzeResult saves the default rule-based analysis on a failed result. A
// successful result, or one that already carries an analysis such as a result
// carried from an earlier run, is returned unchanged. A non-nil readErr means
// the job output could not be read, so the analysis is unavailable.
func AnalyzeResult(result model.JobResult, log string, readErr error) model.JobResult {
	if result.ExitCode == 0 || result.DiagnosisStatus != "" {
		return result
	}
	result.DiagnosisRules = RulesVersion()
	if readErr != nil {
		result.DiagnosisStatus, result.DiagnosisNote = model.DiagnosisUnavailable, readErr.Error()
		return result
	}
	result.Diagnoses = DiagnoseDefault(Job{JobID: result.ID, Error: result.Error, Log: log})
	result.DiagnosisStatus = model.DiagnosisMatched
	if len(result.Diagnoses) == 0 {
		result.DiagnosisStatus = model.DiagnosisNoMatch
	}
	return result
}

// Outdated reports whether a saved matched or no-match analysis was produced
// by rules other than the current ones. An unavailable analysis never ran the
// rules, and an unanalyzed result has nothing to refresh.
func Outdated(result model.JobResult) bool {
	switch result.DiagnosisStatus {
	case model.DiagnosisMatched, model.DiagnosisNoMatch:
		return result.DiagnosisRules != RulesVersion()
	}
	return false
}
