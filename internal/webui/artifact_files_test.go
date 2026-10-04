package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
)

type artifactFixture struct {
	baseDir, runID, work, outside string
	// entries maps a name to its entry index in the recorded listing.
	entries map[string]int
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newArtifactFixture(t *testing.T) artifactFixture {
	t.Helper()
	f := artifactFixture{baseDir: t.TempDir(), runID: "20260922-070308-0d83bd39", work: t.TempDir(), outside: t.TempDir(), entries: map[string]int{}}
	write(t, filepath.Join(f.work, "plot.png"), "\x89PNG-bytes")
	write(t, filepath.Join(f.work, "pic.svg"), `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	write(t, filepath.Join(f.work, "data.csv"), "a,b\n1,2\n")
	var log strings.Builder
	for index := range 20000 {
		fmt.Fprintf(&log, "line %05d\n", index)
	}
	write(t, filepath.Join(f.work, "run.log"), log.String())
	write(t, filepath.Join(f.work, "weights.npy"), "\x93NUMPY\x00\x01")
	for index := range 250 {
		write(t, filepath.Join(f.work, "many", fmt.Sprintf("f%03d.txt", index)), "x")
	}
	if err := os.MkdirAll(filepath.Join(f.work, "many", "zz-sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.outside, "secret.txt"), "secret")
	if err := os.Symlink(filepath.Join(f.outside, "secret.txt"), filepath.Join(f.work, "link.csv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.outside, filepath.Join(f.work, "many", "escape")); err != nil {
		t.Fatal(err)
	}
	paths, err := stateinternal.ResolveProjectPaths(f.baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, f.runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	record := artifact.Record{Version: 7, WorkingDirectory: f.work}
	for index, path := range []string{
		filepath.Join(f.work, "plot.png"), filepath.Join(f.work, "pic.svg"), filepath.Join(f.work, "data.csv"),
		filepath.Join(f.work, "run.log"), filepath.Join(f.work, "weights.npy"), filepath.Join(f.work, "many"),
		filepath.Join(f.outside, "secret.txt"), filepath.Join(f.work, "link.csv"), filepath.Join(f.work, "gone.csv"),
	} {
		f.entries[filepath.Base(path)] = index
		record.Candidates = append(record.Candidates, artifact.Candidate{Path: path, Basis: artifact.BasisAbsolute,
			Sources: []artifact.Source{{Kind: artifact.KindArgument, Value: path, Rule: artifact.RuleExplicitPath}}})
	}
	record.Candidates = append(record.Candidates, artifact.Candidate{Path: "rel.csv", Basis: artifact.BasisUnresolved})
	f.entries["rel.csv"] = len(record.Candidates) - 1
	attemptDir := filepath.Join(runDir, "job-1", "attempts", stateinternal.MakeAttemptID(f.runID, "job-1", 0))
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(attemptDir, stateinternal.ArtifactsFileName), record); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f artifactFixture) get(t *testing.T, options Options, route, name string, extra url.Values) *httptest.ResponseRecorder {
	t.Helper()
	query := url.Values{"project_name": {"default"}, "run_id": {f.runID}, "job_id": {"job-1"}, "entry": {fmt.Sprint(f.entries[name])}}
	for key, values := range extra {
		query[key] = values
	}
	recorder := httptest.NewRecorder()
	Handler(options).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, route+"?"+query.Encode(), nil))
	return recorder
}

func TestArtifactPreviewableFlags(t *testing.T) {
	f := newArtifactFixture(t)
	recorder := httptest.NewRecorder()
	Handler(testOptions(f.baseDir, false)).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/artifacts?project_name=default&run_id="+f.runID+"&job_id=job-1", nil))
	var listing webArtifactListing
	if err := json.Unmarshal(recorder.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	// Files and directories under the working directory are previewable; a
	// path outside it, a missing one, and one without a base are not. A
	// symlink is judged when opened.
	want := []bool{true, true, true, true, true, true, false, true, false, false}
	if fmt.Sprint(listing.Previewable) != fmt.Sprint(want) {
		t.Fatalf("previewable = %v, want %v", listing.Previewable, want)
	}
}

func TestArtifactFileServing(t *testing.T) {
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)

	png := f.get(t, options, "/api/artifact-file", "plot.png", nil)
	if png.Code != http.StatusOK || png.Body.String() != "\x89PNG-bytes" || png.Header().Get("Content-Type") != "image/png" ||
		png.Header().Get("Content-Disposition") != "" {
		t.Fatalf("png = %d %v %q", png.Code, png.Header(), png.Body.String())
	}
	for _, header := range []string{"Cache-Control", "X-Content-Type-Options", "Content-Security-Policy"} {
		if png.Header().Get(header) == "" {
			t.Fatalf("png response lacks %s", header)
		}
	}
	svg := f.get(t, options, "/api/artifact-file", "pic.svg", nil)
	if svg.Code != http.StatusOK || svg.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(svg.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("svg = %d %v", svg.Code, svg.Header())
	}
	csv := f.get(t, options, "/api/artifact-file", "data.csv", nil)
	if csv.Code != http.StatusOK || csv.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(csv.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("non-image file is not a download: %d %v", csv.Code, csv.Header())
	}
	download := f.get(t, options, "/api/artifact-file", "plot.png", url.Values{"download": {"1"}})
	if !strings.Contains(download.Header().Get("Content-Disposition"), `filename="plot.png"`) {
		t.Fatalf("download header = %v", download.Header())
	}
	child := f.get(t, options, "/api/artifact-file", "many", url.Values{"child": {"f001.txt"}})
	if child.Code != http.StatusOK || child.Body.String() != "x" {
		t.Fatalf("child of a listed directory = %d %q", child.Code, child.Body.String())
	}

	for name, test := range map[string]struct {
		entry string
		extra url.Values
		code  int
	}{
		"outside the roots":          {entry: "secret.txt", code: http.StatusForbidden},
		"symlink leaving the root":   {entry: "link.csv", code: http.StatusForbidden},
		"child through a symlink":    {entry: "many", extra: url.Values{"child": {"escape/secret.txt"}}, code: http.StatusForbidden},
		"child with ..":              {entry: "many", extra: url.Values{"child": {"../plot.png"}}, code: http.StatusBadRequest},
		"absolute child":             {entry: "many", extra: url.Values{"child": {"/etc/passwd"}}, code: http.StatusBadRequest},
		"unclean child":              {entry: "many", extra: url.Values{"child": {"zz-sub/../f001.txt"}}, code: http.StatusBadRequest},
		"missing file":               {entry: "gone.csv", code: http.StatusNotFound},
		"no known base":              {entry: "rel.csv", code: http.StatusForbidden},
		"a directory is not a file":  {entry: "many", code: http.StatusBadRequest},
		"entry out of range":         {entry: "plot.png", extra: url.Values{"entry": {"99"}}, code: http.StatusBadRequest},
		"entry that is not a number": {entry: "plot.png", extra: url.Values{"entry": {"../x"}}, code: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			if got := f.get(t, options, "/api/artifact-file", test.entry, test.extra); got.Code != test.code {
				t.Fatalf("status = %d, want %d: %s", got.Code, test.code, got.Body.String())
			}
		})
	}

	options.ArtifactRoots = []string{f.outside}
	if allowed := f.get(t, options, "/api/artifact-file", "secret.txt", nil); allowed.Code != http.StatusOK || allowed.Body.String() != "secret" {
		t.Fatalf("--artifact-root did not allow its file: %d %s", allowed.Code, allowed.Body.String())
	}
	recorder := httptest.NewRecorder()
	Handler(options).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/artifact-file", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST = %d", recorder.Code)
	}
}

func TestArtifactTextPages(t *testing.T) {
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)
	read := func(extra url.Values) artifactText {
		t.Helper()
		recorder := f.get(t, options, "/api/artifact-text", "run.log", extra)
		var page artifactText
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &page) != nil {
			t.Fatalf("text page = %d %s", recorder.Code, recorder.Body.String())
		}
		return page
	}
	last := read(url.Values{"from": {"end"}})
	if last.End != last.Size || last.Start == 0 || !strings.HasPrefix(last.Text, "line ") || !strings.HasSuffix(last.Text, "line 19999\n") {
		t.Fatalf("last page = start %d end %d size %d, text %q...", last.Start, last.End, last.Size, last.Text[:20])
	}
	before := read(url.Values{"from": {"end"}, "offset": {fmt.Sprint(last.Start)}})
	if before.End != last.Start || !strings.HasPrefix(before.Text, "line ") || !strings.HasSuffix(before.Text, "\n") {
		t.Fatalf("earlier page = start %d end %d", before.Start, before.End)
	}
	first := read(url.Values{"from": {"start"}})
	if first.Start != 0 || !strings.HasPrefix(first.Text, "line 00000\n") || !strings.HasSuffix(first.Text, "\n") || first.End >= first.Size {
		t.Fatalf("first page = start %d end %d", first.Start, first.End)
	}
	next := read(url.Values{"from": {"start"}, "offset": {fmt.Sprint(first.End)}})
	if next.Start != first.End || !strings.HasPrefix(next.Text, "line ") {
		t.Fatalf("next page = start %d", next.Start)
	}
	if binary := f.get(t, options, "/api/artifact-text", "weights.npy", nil); binary.Code != http.StatusBadRequest || !strings.Contains(binary.Body.String(), "binary") {
		t.Fatalf("binary file as text = %d %s", binary.Code, binary.Body.String())
	}
	if bad := f.get(t, options, "/api/artifact-text", "run.log", url.Values{"offset": {"-1"}}); bad.Code != http.StatusBadRequest {
		t.Fatalf("negative offset = %d", bad.Code)
	}
}

func TestArtifactDirectoryPages(t *testing.T) {
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)
	read := func(extra url.Values) artifactDirectory {
		t.Helper()
		recorder := f.get(t, options, "/api/artifact-directory", "many", extra)
		var page artifactDirectory
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &page) != nil {
			t.Fatalf("directory page = %d %s", recorder.Code, recorder.Body.String())
		}
		return page
	}
	first := read(nil)
	if first.Total != 252 || len(first.Entries) != artifactDirectoryPage || first.Entries[0].Name != "zz-sub" || first.Entries[0].Type != "directory" ||
		first.Entries[1].Name != "escape" || first.Entries[1].Type != "symlink" || first.Entries[2].Name != "f000.txt" || first.Entries[2].Size != 1 || first.Entries[2].Modified == "" {
		t.Fatalf("first page = total %d, %+v", first.Total, first.Entries[:3])
	}
	second := read(url.Values{"offset": {"200"}})
	if len(second.Entries) != 52 || second.Entries[51].Name != "f249.txt" {
		t.Fatalf("second page = %d entries", len(second.Entries))
	}
	sub := read(url.Values{"child": {"zz-sub"}})
	if sub.Total != 0 || len(sub.Entries) != 0 {
		t.Fatalf("empty child directory = %+v", sub)
	}
	if escaped := f.get(t, options, "/api/artifact-directory", "many", url.Values{"child": {"escape"}}); escaped.Code != http.StatusForbidden {
		t.Fatalf("directory through an escaping symlink = %d %s", escaped.Code, escaped.Body.String())
	}
}
