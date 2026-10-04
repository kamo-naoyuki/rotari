package jobstatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestArtifactsFollowAttemptAndCarriedOrigin(t *testing.T) {
	runsDir := t.TempDir()
	const executed, carried, older = "20260101-000000-aaaaaaaa", "20260102-000000-bbbbbbbb", "20251231-000000-dddddddd"
	first := state.MakeAttemptID(executed, "job", 0)
	retry := state.MakeAttemptID(executed, "job", 1)
	for attemptID, path := range map[string]string{first: "/work/first.csv", retry: "/work/retry.csv"} {
		writeFile(t, filepath.Join(runsDir, executed, "job", "attempts", attemptID, state.ArtifactsFileName),
			`{"version":1,"candidates":[{"path":"`+path+`","basis":"absolute","sources":[{"kind":"argument","value":"`+path+`","rule":"PATH-R2","index":1}]}]}`)
	}
	writeQueue(t, filepath.Join(runsDir, carried), model.JobOrigin{RunID: executed, JobID: "job"})
	// A run written before discovery existed has an attempt without a record.
	oldAttempt := state.MakeAttemptID(older, "job", 0)
	writeFile(t, filepath.Join(runsDir, older, "job", "attempts", oldAttempt, "command.json"), `{"id":"job","command":["true"]}`)

	for name, test := range map[string]struct {
		origin model.JobOrigin
		want   string
	}{
		"latest attempt":       {origin: model.JobOrigin{RunID: executed, JobID: "job"}, want: "/work/retry.csv"},
		"older attempt by ID":  {origin: model.JobOrigin{RunID: executed, JobID: "job", AttemptID: first}, want: "/work/first.csv"},
		"carried result":       {origin: model.JobOrigin{RunID: carried, JobID: "job"}, want: "/work/retry.csv"},
		"carried with attempt": {origin: model.JobOrigin{RunID: carried, JobID: "job", AttemptID: first}, want: "/work/first.csv"},
		"old state":            {origin: model.JobOrigin{RunID: older, JobID: "job"}},
		"missing job":          {origin: model.JobOrigin{RunID: executed, JobID: "other"}},
		"invalid path element": {origin: model.JobOrigin{RunID: "../x", JobID: "job"}},
	} {
		t.Run(name, func(t *testing.T) {
			record, ok := Artifacts(testStore(), runsDir, test.origin)
			if test.want == "" {
				if ok {
					t.Fatalf("Artifacts = %+v, want no record", record)
				}
				return
			}
			if !ok || record.Version != 1 || len(record.Candidates) != 1 || record.Candidates[0].Path != test.want {
				t.Fatalf("Artifacts = %+v, %v, want %s", record, ok, test.want)
			}
			if source := record.Candidates[0].Sources[0]; source.Index == nil || *source.Index != 1 {
				t.Fatalf("source = %+v, want index 1 to round-trip", source)
			}
		})
	}
}

func TestArtifactsIgnoresUnreadableRecord(t *testing.T) {
	runsDir := t.TempDir()
	const runID = "20260101-000000-aaaaaaaa"
	attemptDir := filepath.Join(runsDir, runID, "job", "attempts", state.MakeAttemptID(runID, "job", 0))
	writeFile(t, filepath.Join(attemptDir, state.ArtifactsFileName), "{not json")
	if record, ok := Artifacts(testStore(), runsDir, model.JobOrigin{RunID: runID, JobID: "job"}); ok {
		t.Fatalf("Artifacts = %+v, want no record for a corrupt file", record)
	}
}

func TestListArtifactsObservesEachPath(t *testing.T) {
	runsDir := t.TempDir()
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "a.csv"), "x")
	if err := os.Symlink(filepath.Join(work, "a.csv"), filepath.Join(work, "link.csv")); err != nil {
		t.Fatal(err)
	}
	const runID = "20260101-000000-aaaaaaaa"
	attemptDir := filepath.Join(runsDir, runID, "job", "attempts", state.MakeAttemptID(runID, "job", 0))
	record := artifact.Record{Version: 7, Result: artifact.Result{
		Candidates: []artifact.Candidate{
			{Path: filepath.Join(work, "a.csv"), Basis: artifact.BasisWorkingDirectory, Sources: []artifact.Source{{Kind: artifact.KindArgument, Key: "--in"}}},
			{Path: work, Basis: artifact.BasisAbsolute, Sources: []artifact.Source{{Kind: artifact.KindEnvironment, Key: "OUT_DIR"}}},
			{Path: filepath.Join(work, "link.csv"), Basis: artifact.BasisWorkingDirectory, Sources: []artifact.Source{{Kind: artifact.KindArgument}}},
			{Path: filepath.Join(work, "gone.csv"), Basis: artifact.BasisWorkingDirectory, Sources: []artifact.Source{{Kind: artifact.KindOutput}}},
			{Path: "rel.csv", Basis: artifact.BasisUnresolved, Sources: []artifact.Source{{Kind: artifact.KindArgument}}},
		},
		Diagnostics: []artifact.Diagnostic{{Source: "/x.yaml", Message: "not inspected: no such file"}},
	}}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(attemptDir, state.ArtifactsFileName), string(data))

	listing := ListArtifacts(testStore(), runsDir, model.JobOrigin{RunID: runID, JobID: "job"})
	var got []string
	for _, entry := range listing.Entries {
		got = append(got, entry.Type+" "+filepath.Base(entry.Path)+" "+entry.Origin)
	}
	want := []string{"file a.csv --in", "directory " + filepath.Base(work) + " env OUT_DIR", "file link.csv argument", "missing gone.csv --output", "unknown rel.csv argument"}
	if !listing.Recorded || listing.Version != 7 || !reflect.DeepEqual(got, want) || len(listing.Diagnostics) != 1 {
		t.Fatalf("listing = %+v\nentries %q, want %q", listing, got, want)
	}
	if missing := ListArtifacts(testStore(), runsDir, model.JobOrigin{RunID: runID, JobID: "other"}); missing.Recorded || len(missing.Entries) != 0 {
		t.Fatalf("listing without a record = %+v", missing)
	}
}
