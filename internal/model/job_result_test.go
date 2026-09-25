package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestJobResultUnmarshalConvertsLegacyDiagnosisEntries(t *testing.T) {
	tests := []struct {
		name string
		data string
		want JobResult
	}{
		{
			name: "no match",
			data: `{"id":"job-1","exit_code":1,"diagnoses":[{"name":"No known rule-based diagnosis matched","evidence":"none","suggestion":"inspect"}]}`,
			want: JobResult{ID: "job-1", ExitCode: 1, DiagnosisStatus: DiagnosisNoMatch},
		},
		{
			name: "unavailable",
			data: `{"id":"job-1","exit_code":1,"diagnoses":[{"name":"Rule-based diagnosis unavailable","evidence":"The job output could not be read.","suggestion":"fix"}]}`,
			want: JobResult{ID: "job-1", ExitCode: 1, DiagnosisStatus: DiagnosisUnavailable, DiagnosisNote: "The job output could not be read."},
		},
		{
			name: "matched",
			data: `{"id":"job-1","exit_code":1,"diagnoses":[{"name":"Permission denied","evidence":"permission denied","suggestion":"check"}]}`,
			want: JobResult{ID: "job-1", ExitCode: 1, DiagnosisStatus: DiagnosisMatched, Diagnoses: []RuleDiagnosis{{Name: "Permission denied", Evidence: "permission denied", Suggestion: "check"}}},
		},
		{
			name: "current format",
			data: `{"id":"job-1","exit_code":1,"diagnosis_status":"no_match","diagnosis_rules":"abc"}`,
			want: JobResult{ID: "job-1", ExitCode: 1, DiagnosisStatus: DiagnosisNoMatch, DiagnosisRules: "abc"},
		},
		{
			name: "not analyzed",
			data: `{"id":"job-1","exit_code":0}`,
			want: JobResult{ID: "job-1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result JobResult
			if err := json.Unmarshal([]byte(test.data), &result); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result, test.want) {
				t.Fatalf("Unmarshal() = %#v, want %#v", result, test.want)
			}
		})
	}
}

func TestJobResultClearDiagnosis(t *testing.T) {
	result := JobResult{ID: "job-1", Diagnoses: []RuleDiagnosis{{Name: "rule"}}, DiagnosisStatus: DiagnosisMatched, DiagnosisNote: "note", DiagnosisRules: "abc"}
	result.ClearDiagnosis()
	if !reflect.DeepEqual(result, JobResult{ID: "job-1"}) {
		t.Fatalf("ClearDiagnosis() left %#v", result)
	}
}
