package diagnose

import "testing"

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
