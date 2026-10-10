package pairruns

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIFlagPairNote adds a note to the fixture's finished run with every
// pair of note's options, in both orders, from the same restored state. Each
// invocation must succeed and append exactly that note. Notes record the
// time they were added, so the two orders are compared by their notes'
// text, not byte for byte.
func TestCLIFlagPairNote(t *testing.T) {
	covers(t, "RUN-16")
	f := newPairMutationFixture(t)
	var command pairCommand
	for _, candidate := range readPairSchema(t, f.E) {
		if candidate.Name == "note" {
			command = candidate
			break
		}
	}
	if command.Name == "" {
		t.Fatal("note is missing from the schema")
	}
	for _, pair := range commandFlagPairs(command) {
		t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
			for _, order := range [][]pairFlag{{pair.A, pair.B}, {pair.B, pair.A}} {
				f.Initial.Restore(t, f.E.Root, *f.Current)
				text := "pair note " + order[0].Name + " " + order[1].Name
				args := []string{"note"}
				for _, flag := range order {
					args = append(args, f.Sample(t, flag)...)
				}
				result := pairInvoke(t, f.E, append(args, f.Run, text)...)
				// Restore needs the state this invocation left.
				*f.Current = savePairTree(t, f.E.Root)
				if result.Code != 0 {
					assertPairOutcome(t, result)
					t.Fatalf("note failed: %s", result)
				}
				data, err := os.ReadFile(filepath.Join(f.E.Base, "projects", f.Project, "runs", f.Run, "notes.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(data)), "\n")
				var note struct {
					Text string `json:"text"`
				}
				if len(lines) != 1 || json.Unmarshal([]byte(lines[0]), &note) != nil || note.Text != text {
					t.Fatalf("notes after %v = %q, want only %q", args, data, text)
				}
			}
		})
	}
}
