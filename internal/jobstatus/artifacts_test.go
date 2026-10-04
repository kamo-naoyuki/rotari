package jobstatus

import (
	"path/filepath"
	"testing"

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
