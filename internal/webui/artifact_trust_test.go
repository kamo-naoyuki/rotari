package webui

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
)

func TestBasedirParentRoot(t *testing.T) {
	for baseDir, want := range map[string]string{
		"/data/exp/.rotari-state": "/data/exp",
		"/data/exp/state/":        "/data/exp",
		"/state":                  "",
		"relative/state":          "",
	} {
		got, ok := basedirParentRoot(baseDir)
		if (want == "") == ok || got != want && ok {
			t.Errorf("basedirParentRoot(%q) = %q, %v; want %q", baseDir, got, ok, want)
		}
	}
}

// addBesideCandidate records a file next to the fixture's basedir, outside
// the job's working directory, as one more candidate of job-1's attempt.
func addBesideCandidate(t *testing.T, f artifactFixture) (string, int) {
	t.Helper()
	beside := filepath.Join(filepath.Dir(f.baseDir), "outputs", "beside.csv")
	write(t, beside, "x,y\n")
	paths, err := stateinternal.ResolveProjectPaths(f.baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(paths.RunsDir, f.runID, "job-1", "attempts", stateinternal.MakeAttemptID(f.runID, "job-1", 0), stateinternal.ArtifactsFileName)
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	var recorded artifact.Record
	if err := json.Unmarshal(data, &recorded); err != nil {
		t.Fatal(err)
	}
	recorded.Candidates = append(recorded.Candidates, artifact.Candidate{Path: beside, Basis: artifact.BasisAbsolute,
		Sources: []artifact.Source{{Kind: artifact.KindArgument, Value: beside, Rule: artifact.RuleExplicitPath}}})
	if err := stateinternal.WriteJSON(record, recorded); err != nil {
		t.Fatal(err)
	}
	return beside, len(recorded.Candidates) - 1
}

func artifactsListing(t *testing.T, f artifactFixture, options Options) webArtifactListing {
	t.Helper()
	recorder := fetchBody(t, options, "/api/artifacts?"+url.Values{"project_name": {"default"}, "run_id": {f.runID}, "job_id": {"job-1"}}.Encode())
	var listing webArtifactListing
	if err := json.Unmarshal(recorder, &listing); err != nil {
		t.Fatalf("decode listing %s: %v", recorder, err)
	}
	return listing
}

func fetchBody(t *testing.T, options Options, target string) []byte {
	t.Helper()
	recorder := serveJSON(t, Handler(options), http.MethodGet, target, "")
	return recorder.Body.Bytes()
}

func TestArtifactRootsFollowRecordTrust(t *testing.T) {
	f := newArtifactFixture(t)
	beside, besideIndex := addBesideCandidate(t, f)
	options := testOptions(f.baseDir, false)

	// A basedir only this user can write: the working directory and the
	// basedir's parent are served.
	listing := artifactsListing(t, f, options)
	if listing.RootsNote != "" || !listing.Previewable[f.entries["data.csv"]] || !listing.Previewable[besideIndex] {
		t.Fatalf("trusted listing = note %q, previewable %v", listing.RootsNote, listing.Previewable)
	}
	served := f.get(t, options, "/api/artifact-file", "", url.Values{"entry": {itoa(besideIndex)}})
	if served.Code != http.StatusOK || served.Body.String() != "x,y\n" {
		t.Fatalf("file beside the basedir = %d %q", served.Code, served.Body.String())
	}

	// A project another user could write: nothing it recorded is served
	// except under --artifact-root, and the listing says why.
	paths, err := stateinternal.ResolveProjectPaths(f.baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.ProjectDir, 0o775); err != nil {
		t.Fatal(err)
	}
	listing = artifactsListing(t, f, options)
	if !strings.Contains(listing.RootsNote, paths.ProjectDir+" is writable by group or others") ||
		listing.Previewable[f.entries["data.csv"]] || listing.Previewable[besideIndex] {
		t.Fatalf("untrusted listing = note %q, previewable %v", listing.RootsNote, listing.Previewable)
	}
	refused := f.get(t, options, "/api/artifact-file", "data.csv", nil)
	if refused.Code != http.StatusForbidden || !strings.Contains(refused.Body.String(), "another user could have chosen its paths") {
		t.Fatalf("untrusted working directory file = %d %q", refused.Code, refused.Body.String())
	}
	options.ArtifactRoots = []string{f.work}
	if allowed := f.get(t, options, "/api/artifact-file", "data.csv", nil); allowed.Code != http.StatusOK {
		t.Fatalf("untrusted file under --artifact-root = %d %q", allowed.Code, allowed.Body.String())
	}
	if besideFile := f.get(t, options, "/api/artifact-file", "", url.Values{"entry": {itoa(besideIndex)}}); besideFile.Code != http.StatusForbidden {
		t.Fatalf("untrusted file beside the basedir = %d, want 403 (%s)", besideFile.Code, beside)
	}

	// Each element counts, down to the run directory.
	if err := os.Chmod(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(paths.RunsDir, f.runID), 0o757); err != nil {
		t.Fatal(err)
	}
	if note := artifactsListing(t, f, testOptions(f.baseDir, false)).RootsNote; !strings.Contains(note, f.runID+" is writable by group or others") {
		t.Fatalf("run directory writable by others: note %q", note)
	}
}

func itoa(value int) string {
	data, _ := json.Marshal(value)
	return string(data)
}
