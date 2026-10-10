package queueops

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestAddNote(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20261010-090000-aaaaaaaa"
	attemptID := state.MakeAttemptID(runID, "job1", 0)
	attemptDir, err := state.SpecificAttemptJobDir(paths.RunsDir+"/"+runID, "job1", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 10, 9, 30, 0, 0, time.UTC)
	if note, err := AddNote(paths, runID, "", "  why this run ", now); err != nil || note.Text != "why this run" || note.JobID != "" || note.At != "2026-10-10T09:30:00Z" {
		t.Fatalf("run note = %+v, %v", note, err)
	}
	if note, err := AddNote(paths, runID, attemptID, "NaN is expected", now); err != nil || note.JobID != "job1" || note.AttemptID != attemptID {
		t.Fatalf("attempt note = %+v, %v", note, err)
	}
	for _, test := range []struct {
		name, runID, attemptID, text, want string
	}{
		{"empty text", runID, "", " ", "must not be empty"},
		{"unknown run", "20261010-090000-bbbbbbbb", "", "x", "not found"},
		{"attempt of another run", runID, state.MakeAttemptID("20261010-090000-bbbbbbbb", "job1", 0), "x", "belongs to run"},
		{"unknown attempt", runID, state.MakeAttemptID(runID, "job1", 1), "x", "not found"},
		{"path in the run ID", "../x", "", "x", "invalid"},
	} {
		if _, err := AddNote(paths, test.runID, test.attemptID, test.text, now); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s: AddNote error = %v, want %q", test.name, err, test.want)
		}
	}
	notes, err := state.LoadRunNotes(paths.RunsDir + "/" + runID)
	if err != nil || len(notes) != 2 {
		t.Fatalf("notes after the failed adds = %+v, %v; want the two added", notes, err)
	}
}
