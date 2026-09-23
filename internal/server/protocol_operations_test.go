package server

import "testing"

func TestIsKnownOperation(t *testing.T) {
	for _, operation := range []string{OpPing, OpShutdown, OpRun, OpSubmit, OpCancel, OpSuspend, OpResume, OpCopy, OpChange, OpRemove, OpClear} {
		if !IsKnownOperation(operation) {
			t.Errorf("IsKnownOperation(%q) = false", operation)
		}
	}
	if IsKnownOperation("unknown") {
		t.Fatal("IsKnownOperation(unknown) = true")
	}
}
