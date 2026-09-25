package diagnose

import (
	"errors"
	"regexp"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestAnalyzeResultRecordsStatusAndRules(t *testing.T) {
	matched := AnalyzeResult(model.JobResult{ID: "job-1", ExitCode: 1}, "CUDA out of memory", nil)
	if matched.DiagnosisStatus != model.DiagnosisMatched || len(matched.Diagnoses) != 1 || matched.DiagnosisRules != RulesVersion() {
		t.Fatalf("AnalyzeResult(match) = %#v", matched)
	}
	noMatch := AnalyzeResult(model.JobResult{ID: "job-1", ExitCode: 1}, "ordinary failure", nil)
	if noMatch.DiagnosisStatus != model.DiagnosisNoMatch || len(noMatch.Diagnoses) != 0 || noMatch.DiagnosisRules != RulesVersion() {
		t.Fatalf("AnalyzeResult(no match) = %#v", noMatch)
	}
	unavailable := AnalyzeResult(model.JobResult{ID: "job-1", ExitCode: 1}, "CUDA out of memory", errors.New("output unreadable"))
	if unavailable.DiagnosisStatus != model.DiagnosisUnavailable || unavailable.DiagnosisNote != "output unreadable" || len(unavailable.Diagnoses) != 0 {
		t.Fatalf("AnalyzeResult(unavailable) = %#v", unavailable)
	}
}

func TestAnalyzeResultKeepsSuccessAndSavedAnalysis(t *testing.T) {
	success := model.JobResult{ID: "job-1"}
	if got := AnalyzeResult(success, "CUDA out of memory", nil); got.DiagnosisStatus != "" {
		t.Fatalf("AnalyzeResult(success) = %#v, want no analysis", got)
	}
	saved := model.JobResult{ID: "job-1", ExitCode: 1, DiagnosisStatus: model.DiagnosisNoMatch, DiagnosisRules: "old"}
	if got := AnalyzeResult(saved, "CUDA out of memory", nil); got.DiagnosisStatus != model.DiagnosisNoMatch || got.DiagnosisRules != "old" {
		t.Fatalf("AnalyzeResult(saved) = %#v, want saved analysis kept", got)
	}
}

func TestOutdatedComparesRulesOfRuleOutcomes(t *testing.T) {
	tests := []struct {
		result model.JobResult
		want   bool
	}{
		{result: model.JobResult{DiagnosisStatus: model.DiagnosisMatched, DiagnosisRules: RulesVersion()}},
		{result: model.JobResult{DiagnosisStatus: model.DiagnosisMatched, DiagnosisRules: "old"}, want: true},
		{result: model.JobResult{DiagnosisStatus: model.DiagnosisNoMatch}, want: true},
		{result: model.JobResult{DiagnosisStatus: model.DiagnosisUnavailable, DiagnosisRules: "old"}},
		{result: model.JobResult{}},
	}
	for _, test := range tests {
		if got := Outdated(test.result); got != test.want {
			t.Fatalf("Outdated(%#v) = %v, want %v", test.result, got, test.want)
		}
	}
}

func TestRulesVersionChangesWithRuleDefinitions(t *testing.T) {
	version := RulesVersion()
	if len(version) != 12 {
		t.Fatalf("RulesVersion() = %q, want 12 hex characters", version)
	}
	saved := defaultRules
	t.Cleanup(func() { defaultRules = saved })
	defaultRules = append(append([]Rule(nil), saved...), Rule{Name: "extra", Patterns: []*regexp.Regexp{regexp.MustCompile("extra")}})
	if got := computeRulesVersion(); got == version {
		t.Fatalf("rules version %q did not change after adding a rule", got)
	}
}
