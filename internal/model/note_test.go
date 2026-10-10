package model

import (
	"strings"
	"testing"
	"time"
)

func TestNoteText(t *testing.T) {
	if text, err := NoteText("  retry after the fix \n"); err != nil || text != "retry after the fix" {
		t.Fatalf("NoteText trims to %q, %v", text, err)
	}
	for _, bad := range []string{"", " \n\t", strings.Repeat("x", MaxNoteLength+1)} {
		if _, err := NoteText(bad); err == nil {
			t.Errorf("NoteText(%d characters) accepted an invalid note", len(bad))
		}
	}
	if _, err := NoteText(strings.Repeat("あ", MaxNoteLength)); err != nil {
		t.Errorf("NoteText counted bytes instead of characters: %v", err)
	}
}

func TestRunNotesForSeparatesRunAndJobNotes(t *testing.T) {
	notes := []RunNote{{Text: "run"}, {JobID: "a", Text: "a1"}, {JobID: "b", Text: "b"}, {JobID: "a", Text: "a2"}}
	if got := RunNotesFor(notes, ""); len(got) != 1 || got[0].Text != "run" {
		t.Errorf("run notes = %+v", got)
	}
	if got := RunNotesFor(notes, "a"); len(got) != 2 || got[0].Text != "a1" || got[1].Text != "a2" {
		t.Errorf("job a notes = %+v", got)
	}
}

func TestFormatRunNote(t *testing.T) {
	at := "2026-10-10T09:00:00Z"
	stamp := FormatDisplayTimestampIn(at, time.Local)
	for _, test := range []struct {
		note  RunNote
		label string
		want  string
	}{
		{RunNote{At: at, Text: "first sweep"}, "", stamp + " first sweep"},
		{RunNote{At: at, JobID: "j1", Text: "NaN is expected"}, "train[3]", stamp + " [train[3]] NaN is expected"},
		{RunNote{At: at, JobID: "j1", Text: "no name"}, "", stamp + " [j1] no name"},
		{RunNote{At: at, Text: "two\nlines"}, "", stamp + " two\n  lines"},
	} {
		if got := FormatRunNote(test.note, test.label); got != test.want {
			t.Errorf("FormatRunNote(%+v, %q) = %q, want %q", test.note, test.label, got, test.want)
		}
	}
}
