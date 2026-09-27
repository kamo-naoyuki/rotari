package server

import "testing"

func TestIsKnownOperation(t *testing.T) {
	if !IsKnownOperation(OpRun) {
		t.Errorf("IsKnownOperation(%q) = false", OpRun)
	}
	for _, operation := range []string{"ping", "shutdown", "unknown"} {
		if IsKnownOperation(operation) {
			t.Errorf("IsKnownOperation(%q) = true", operation)
		}
	}
}
