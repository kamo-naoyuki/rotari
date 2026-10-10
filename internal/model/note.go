package model

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// RunNote is free text that an agent or a person attached to a run, or to
// one job attempt of it: why the run was made, or what its result showed.
// Notes are only ever added, never edited, so a finished run keeps its
// results unchanged.
type RunNote struct {
	At        string `json:"at"`
	JobID     string `json:"job_id,omitempty"`
	AttemptID string `json:"attempt_id,omitempty"`
	Text      string `json:"text"`
}

// MaxNoteLength is the most characters a note may have.
const MaxNoteLength = 4000

// NoteText trims a note and checks that it is neither empty nor longer than
// MaxNoteLength.
func NoteText(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("a note must not be empty")
	}
	if length := utf8.RuneCountInString(trimmed); length > MaxNoteLength {
		return "", fmt.Errorf("a note may have at most %d characters, not %d", MaxNoteLength, length)
	}
	return trimmed, nil
}

// RunNotesFor returns the notes on the run itself, with no job, when jobID
// is empty, and otherwise the notes on that job's attempts.
func RunNotesFor(notes []RunNote, jobID string) []RunNote {
	selected := make([]RunNote, 0, len(notes))
	for _, note := range notes {
		if note.JobID == jobID {
			selected = append(selected, note)
		}
	}
	return selected
}

// FormatRunNote describes a note for display: when it was written, the job
// it is about (jobLabel, such as the job's name, for a job note), and its
// text, with each further line indented to line up under the first.
func FormatRunNote(note RunNote, jobLabel string) string {
	prefix := FormatDisplayTimestamp(note.At)
	if note.JobID != "" {
		if jobLabel == "" {
			jobLabel = note.JobID
		}
		prefix += " [" + jobLabel + "]"
	}
	return prefix + " " + strings.ReplaceAll(note.Text, "\n", "\n  ")
}
