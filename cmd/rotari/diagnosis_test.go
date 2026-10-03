package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestTailStringKeepsLogEnd(t *testing.T) {
	got := diagnose.TailLog("0123456789", 4)
	if !strings.HasPrefix(got, "[earlier log output omitted]") || !strings.HasSuffix(got, "6789") {
		t.Fatalf("diagnose.TailLog() = %q", got)
	}
}

func TestDiagnoseJobResultReadsOutputAndPreservesSavedDiagnoses(t *testing.T) {
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, state.StderrFileName), []byte("CUDA out of memory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := diagnoseJobResult(runDir, model.JobResult{ID: "job-1", ExitCode: 1})
	if len(result.Diagnoses) != 1 || result.Diagnoses[0].Name != "CUDA/GPU memory exhausted" || result.DiagnosisStatus != model.DiagnosisMatched || result.DiagnosisRules != diagnose.RulesVersion() {
		t.Fatalf("diagnoseJobResult() = %#v", result)
	}

	saved := model.JobResult{ID: "job-1", ExitCode: 1, Diagnoses: []model.RuleDiagnosis{{Name: "Saved diagnosis"}}, DiagnosisStatus: model.DiagnosisMatched}
	if got := diagnoseJobResult(runDir, saved); len(got.Diagnoses) != 1 || got.Diagnoses[0].Name != "Saved diagnosis" {
		t.Fatalf("diagnoseJobResult() overwrote saved diagnoses: %#v", got.Diagnoses)
	}
}

func TestDiagnoseJobResultPersistsNoMatch(t *testing.T) {
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, state.StdoutFileName), []byte("unrecognized failure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := diagnoseJobResult(runDir, model.JobResult{ID: "job-1", ExitCode: 1})
	if len(result.Diagnoses) != 0 || result.DiagnosisStatus != model.DiagnosisNoMatch {
		t.Fatalf("diagnoseJobResult() = %#v, want no-match status without diagnoses", result)
	}
}

func TestDiagnoseJobResultRejectsInvalidJobIDWithUnavailableDiagnosis(t *testing.T) {
	result := diagnoseJobResult(t.TempDir(), model.JobResult{ID: "../outside", ExitCode: 1, Error: "bad input"})
	if len(result.Diagnoses) != 0 || result.DiagnosisStatus != model.DiagnosisUnavailable {
		t.Fatalf("result = %#v, want unavailable status for invalid job ID", result)
	}
	if !strings.Contains(result.DiagnosisNote, "job ID is invalid") {
		t.Fatalf("note = %q, want invalid job ID message", result.DiagnosisNote)
	}
}
