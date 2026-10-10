package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestRunNotesAppendInOrder(t *testing.T) {
	runDir := t.TempDir()
	if notes, err := LoadRunNotes(runDir); err != nil || len(notes) != 0 {
		t.Fatalf("LoadRunNotes of a run without notes = %+v, %v", notes, err)
	}
	written := []model.RunNote{{At: "t1", Text: "why"}, {At: "t2", JobID: "j", AttemptID: "att_x", Text: "line one\nline two"}}
	for _, note := range written {
		if err := AppendRunNote(runDir, note); err != nil {
			t.Fatal(err)
		}
	}
	notes, err := LoadRunNotes(runDir)
	if err != nil || len(notes) != 2 || notes[0] != written[0] || notes[1] != written[1] {
		t.Fatalf("LoadRunNotes = %+v, %v; want %+v", notes, err, written)
	}
}

func TestLoadRunNotesReportsAMalformedLine(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, RunNotesFileName), []byte("{\"text\":\"ok\"}\nnot json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunNotes(runDir); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("LoadRunNotes error = %v, want one naming line 2", err)
	}
}
