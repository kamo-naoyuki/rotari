package diagnose

import (
	"strings"
	"testing"
)

func TestBuildPromptIncludesJobContext(t *testing.T) {
	exitCode := 17
	prompt := BuildPrompt(Job{
		RunID:    "run-1",
		JobID:    "job-2",
		Command:  []string{"python", "train.py"},
		ExitCode: &exitCode,
		Error:    "scheduler error",
		Log:      "traceback",
	}, "ja-JP")

	for _, want := range []string{
		`Respond in the language identified by the BCP 47 tag "ja-JP".`,
		"Run: run-1",
		"Job: job-2",
		"Status: exit code 17",
		"Scheduler error: scheduler error",
		"Command: python train.py",
		"Log tail:\ntraceback",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("BuildPrompt() does not contain %q: %q", want, prompt)
		}
	}
}

func TestBuildPromptUsesNotRecordedWithoutExitCode(t *testing.T) {
	prompt := BuildPrompt(Job{RunID: "run-1", JobID: "job-1"}, "")
	if !strings.Contains(prompt, "Status: not recorded") {
		t.Fatalf("BuildPrompt() = %q", prompt)
	}
	if strings.Contains(prompt, "BCP 47") {
		t.Fatalf("BuildPrompt() added language instruction without a language: %q", prompt)
	}
}

func TestIsLanguageTag(t *testing.T) {
	tests := map[string]bool{
		"ja":      true,
		"en-US":   true,
		"zh-Hant": true,
		"":        false,
		"j":       false,
		"english": false,
		"ja_JP":   false,
		"ja--JP":  false,
		"ja-日本語":  false,
	}
	for value, want := range tests {
		if got := IsLanguageTag(value); got != want {
			t.Errorf("IsLanguageTag(%q) = %t, want %t", value, got, want)
		}
	}
}

func TestTailLog(t *testing.T) {
	if got := TailLog("short", 10); got != "short" {
		t.Errorf("TailLog(short) = %q", got)
	}
	if got := TailLog("0123456789", 4); got != "[earlier log output omitted]\n6789" {
		t.Errorf("TailLog(long) = %q", got)
	}
}

func TestDiagnoseDefaultMatchesSchedulerErrorAndLog(t *testing.T) {
	diagnoses := DiagnoseDefault(Job{
		Error: "CUDA error: out of memory",
		Log:   "Traceback (most recent call last):\nValueError: invalid batch size",
	})
	if len(diagnoses) != 2 {
		t.Fatalf("DiagnoseDefault() returned %d diagnoses: %#v", len(diagnoses), diagnoses)
	}
	if diagnoses[0].Name != "CUDA/GPU memory exhausted" {
		t.Errorf("first diagnosis = %#v", diagnoses[0])
	}
	if diagnoses[1].Name != "Python type or value error" || diagnoses[1].Evidence != "ValueError: invalid batch size" {
		t.Errorf("second diagnosis = %#v", diagnoses[1])
	}
}

func TestDefaultRulesReturnsIndependentSlice(t *testing.T) {
	rules := DefaultRules()
	rules[0].Name = "changed"
	if DefaultRules()[0].Name == "changed" {
		t.Fatal("DefaultRules() returned a slice backed by mutable package state")
	}
}
