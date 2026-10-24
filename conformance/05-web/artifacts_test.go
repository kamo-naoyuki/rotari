package web

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

func TestWebShowsArtifactCandidates(t *testing.T) {
	covers(t, "WEB-5", "WEB-6")
	e := support.NewEnv(t)
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "art", "--", "sh", "-c", "echo x > out.txt; exit 3", "sh", "missing.csv"))
	bareJobID := support.AddedJobID(t, e.MustRotari("add", "-p", "art", "--", "true"))
	e.Rotari("run", "-p", "art", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
		Jobs  []struct {
			Artifacts json.RawMessage `json:"artifacts"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "art", "-j", jobID, "--json").Stdout), &shown); err != nil || len(shown.Jobs) != 1 {
		t.Fatalf("show --json: %v", err)
	}
	var runShown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "art", "--json").Stdout), &runShown); err != nil {
		t.Fatal(err)
	}
	var fromCLI any
	if err := json.Unmarshal(shown.Jobs[0].Artifacts, &fromCLI); err != nil {
		t.Fatal(err)
	}

	base := e.StartWeb()
	query := url.Values{"project_name": {"art"}, "run_id": {runShown.RunID}, "job_id": {jobID}}
	response := e.HTTPGet(base + "/api/artifacts?" + query.Encode())
	if response.Status != 200 {
		t.Fatalf("/api/artifacts = (%d, %s)", response.Status, response.Body)
	}
	var fromWeb map[string]any
	if err := json.Unmarshal([]byte(response.Body), &fromWeb); err != nil {
		t.Fatal(err)
	}
	// The Web adds whether the live server previews each entry (WEB-6).
	if previewable, ok := fromWeb["previewable"].([]any); !ok || len(previewable) != 2 || previewable[0] != true || previewable[1] != false {
		t.Fatalf("previewable = %v", fromWeb["previewable"])
	}
	// So does whether the working directory is served as entry "wd".
	if fromWeb["working_directory_previewable"] != true || fromWeb["working_directory_type"] != "directory" {
		t.Fatalf("working directory = %v (%v)", fromWeb["working_directory_type"], fromWeb["working_directory_previewable"])
	}
	delete(fromWeb, "previewable")
	delete(fromWeb, "working_directory_previewable")
	if !reflect.DeepEqual(any(fromWeb), fromCLI) {
		t.Fatalf("Web and CLI listings differ:\nweb %s\ncli %s", response.Body, shown.Jobs[0].Artifacts)
	}
	if !strings.Contains(response.Body, `"display_path":"out.txt","basis":"working_directory","type":"file"`) ||
		!strings.Contains(response.Body, `"display_path":"missing.csv","basis":"working_directory","type":"missing"`) {
		t.Fatalf("listing lacks the expected entries: %s", response.Body)
	}
	// A job that names no paths still has its working directory, so the
	// live server says it serves entries, of which there are none (WEB-6).
	query.Set("job_id", bareJobID)
	bare := e.HTTPGet(base + "/api/artifacts?" + query.Encode())
	var bareListing map[string]any
	if err := json.Unmarshal([]byte(bare.Body), &bareListing); err != nil || bare.Status != 200 {
		t.Fatalf("/api/artifacts for a job without candidates = (%d, %s)", bare.Status, bare.Body)
	}
	if previewable, ok := bareListing["previewable"].([]any); !ok || len(previewable) != 0 || bareListing["entries"] != nil ||
		bareListing["working_directory_previewable"] != true {
		t.Fatalf("listing without candidates = %s", bare.Body)
	}
	query.Set("job_id", "nope")
	if rejected := e.HTTPGet(base + "/api/artifacts?" + query.Encode()); rejected.Status == 200 {
		t.Fatalf("unknown job accepted: %s", rejected.Body)
	}

	output := filepath.Join(e.Root, "static")
	e.MustRotari("web", "--static-dir", output)
	page, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`window\.__ROTARI_STATIC_ARTIFACTS__ = (\{.*\});`).FindSubmatch(page)
	if match == nil {
		t.Fatal("static export has no artifact listings")
	}
	var embedded map[string]any
	if err := json.Unmarshal(match[1], &embedded); err != nil {
		t.Fatal(err)
	}
	if listing, ok := embedded["art/"+runShown.RunID+"/"+jobID+"/"]; !ok || !reflect.DeepEqual(listing, fromCLI) {
		t.Fatalf("static listing = %v, want the CLI's %v", listing, fromCLI)
	}
}

func TestWebPreviewsArtifactsUnderAllowedRoots(t *testing.T) {
	covers(t, "WEB-6")
	e := support.NewEnv(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `printf 'PNG' > plot.png; mkdir -p d; echo a > d/x.txt; seq 1 5 > n.log; ln -s "$1" link.txt`
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "pv", "--", "sh", "-c", script, "sh",
		filepath.Join(outside, "secret.txt"), "plot.png", "d/", "n.log", "link.txt"))
	e.MustRotari("run", "-p", "pv", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "pv", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	entries := map[string]int{}
	var parsed struct {
		Jobs []struct {
			Artifacts struct {
				Entries []struct {
					DisplayPath string `json:"display_path"`
				} `json:"entries"`
			} `json:"artifacts"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "pv", "-j", jobID, "--json").Stdout), &parsed); err != nil {
		t.Fatal(err)
	}
	for index, entry := range parsed.Jobs[0].Artifacts.Entries {
		entries[filepath.Base(entry.DisplayPath)] = index
	}
	get := func(base, route, name string, extra url.Values) support.HTTPResult {
		query := url.Values{"project_name": {"pv"}, "run_id": {shown.RunID}, "job_id": {jobID}, "entry": {strconv.Itoa(entries[name])}}
		for key, values := range extra {
			query[key] = values
		}
		return e.HTTPGet(base + route + "?" + query.Encode())
	}

	base := e.StartWeb()
	if r := get(base, "/api/artifact-file", "plot.png", nil); r.Status != 200 || r.Body != "PNG" {
		t.Fatalf("image under the working directory = (%d, %q)", r.Status, r.Body)
	}
	for name, extra := range map[string]url.Values{
		"secret.txt": nil,                        // outside every allowed root
		"link.txt":   nil,                        // a symlink leaving the root
		"d":          {"child": {"../plot.png"}}, // a child path leaving the directory
	} {
		if r := get(base, "/api/artifact-file", name, extra); r.Status == 200 {
			t.Fatalf("%s %v was served: %s", name, extra, r.Body)
		}
	}
	if r := get(base, "/api/artifact-text", "n.log", url.Values{"from": {"end"}}); r.Status != 200 || !strings.Contains(r.Body, `"text":"1\n2\n3\n4\n5\n"`) {
		t.Fatalf("text page = (%d, %s)", r.Status, r.Body)
	}
	if r := get(base, "/api/artifact-directory", "d", nil); r.Status != 200 || !strings.Contains(r.Body, `"name":"x.txt","type":"file","size":2`) {
		t.Fatalf("directory page = (%d, %s)", r.Status, r.Body)
	}
	if r := get(base, "/api/artifact-file", "d", url.Values{"child": {"x.txt"}, "download": {"1"}}); r.Status != 200 || r.Body != "a\n" {
		t.Fatalf("child download = (%d, %q)", r.Status, r.Body)
	}
	// The working directory is entry "wd": browsable, with its children served.
	wd := func(route string, extra url.Values) support.HTTPResult {
		query := url.Values{"project_name": {"pv"}, "run_id": {shown.RunID}, "job_id": {jobID}, "entry": {"wd"}}
		for key, values := range extra {
			query[key] = values
		}
		return e.HTTPGet(base + route + "?" + query.Encode())
	}
	if r := wd("/api/artifact-directory", nil); r.Status != 200 || !strings.Contains(r.Body, `"name":"plot.png"`) || !strings.Contains(r.Body, `"name":"d","type":"directory"`) {
		t.Fatalf("working directory page = (%d, %s)", r.Status, r.Body)
	}
	if r := wd("/api/artifact-file", url.Values{"child": {"d/x.txt"}, "download": {"1"}}); r.Status != 200 || r.Body != "a\n" {
		t.Fatalf("working directory child = (%d, %q)", r.Status, r.Body)
	}
	if r := wd("/api/artifact-file", url.Values{"child": {"link.txt"}, "download": {"1"}}); r.Status == 200 {
		t.Fatalf("a symlink out of the working directory was served: %s", r.Body)
	}

	extended := e.StartWeb("--artifact-root", outside)
	if r := get(extended, "/api/artifact-file", "secret.txt", url.Values{"download": {"1"}}); r.Status != 200 || r.Body != "secret" {
		t.Fatalf("--artifact-root file = (%d, %q)", r.Status, r.Body)
	}
	if r := get(extended, "/api/artifact-file", "link.txt", url.Values{"download": {"1"}}); r.Status == 200 {
		t.Fatalf("a symlink out of the working directory was served though its target is under another root: %s", r.Body)
	}
	if r := e.Rotari("web", "--artifact-root", filepath.Join(outside, "secret.txt"), "--port", "0"); r.Code != 1 || !strings.Contains(r.Stderr, "is not a directory") {
		t.Fatalf("--artifact-root of a file = %s", r)
	}
}

func TestStaticExportCopiesArtifactContents(t *testing.T) {
	covers(t, "WEB-7")
	e := support.NewEnv(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `printf 'PNG' > plot.png; seq 1 3 > n.log; ln -s "$1" link.txt`
	e.MustRotari("add", "-p", "st", "--", "sh", "-c", script, "sh", filepath.Join(outside, "secret.txt"), "plot.png", "n.log", "link.txt")
	e.MustRotari("run", "-p", "st", "--quiet")

	plain := filepath.Join(e.Root, "plain")
	e.MustRotari("web", "--static-dir", plain)
	if _, err := os.Stat(filepath.Join(plain, "artifact-files")); !os.IsNotExist(err) {
		t.Fatalf("a static export without the flag copied files: %v", err)
	}

	output := filepath.Join(e.Root, "static")
	result := e.MustRotari("web", "--static-dir", output, "--static-artifact-contents")
	if !strings.Contains(result.Stderr, "Copied 2 artifact files (9 bytes)") {
		t.Fatalf("copy notice = %q", result.Stderr)
	}
	files, err := os.ReadDir(filepath.Join(output, "artifact-files"))
	if err != nil || len(files) != 2 {
		t.Fatalf("artifact-files = %v, %v; want plot.png and n.log only", files, err)
	}
	page, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`window\.__ROTARI_STATIC_ARTIFACT_CONTENTS__ =\s*(\{.*\});`).FindSubmatch(page)
	if match == nil {
		t.Fatal("static export has no artifact contents")
	}
	var contents struct {
		Files map[string]string          `json:"files"`
		Pages map[string]json.RawMessage `json:"pages"`
	}
	if err := json.Unmarshal(match[1], &contents); err != nil {
		t.Fatal(err)
	}
	copied := map[string]string{}
	for _, path := range contents.Files {
		data, err := os.ReadFile(filepath.Join(output, path))
		if err != nil {
			t.Fatal(err)
		}
		copied[filepath.Ext(path)] = string(data)
	}
	if copied[".png"] != "PNG" || copied[".log"] != "1\n2\n3\n" {
		t.Fatalf("copied contents = %q", copied)
	}
	textPage := false
	for key, value := range contents.Pages {
		textPage = textPage || (strings.HasSuffix(key, "text") && strings.Contains(string(value), `"text":"1\n2\n3\n"`))
	}
	if !textPage {
		t.Fatalf("no embedded text page: %v", contents.Pages)
	}
	if r := e.Rotari("web", "--static-artifact-contents", "--port", "0"); r.Code != 1 || !strings.Contains(r.Stderr, "requires --static-dir") {
		t.Fatalf("flag without --static-dir = %s", r)
	}
}

// TestWebServesArtifactsOnlyFromTrustedRecords checks WEB-6's roots: a
// project only this user can write also serves files beside its basedir;
// one another user could write serves only --artifact-root, and says why.
func TestWebServesArtifactsOnlyFromTrustedRecords(t *testing.T) {
	covers(t, "WEB-6")
	e := support.NewEnv(t)
	beside := filepath.Join(e.Root, "outputs", "beside.txt")
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "tr", "--artifact", beside, "--artifact", "work.txt",
		"--", "sh", "-c", `mkdir -p outputs && echo out > outputs/beside.txt && echo work > work.txt`))
	e.MustRotari("run", "-p", "tr", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "tr", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	base := e.StartWeb()
	query := url.Values{"project_name": {"tr"}, "run_id": {shown.RunID}, "job_id": {jobID}}
	var listing struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
		Previewable []bool `json:"previewable"`
		RootsNote   string `json:"roots_note"`
	}
	read := func(base string) {
		t.Helper()
		if err := json.Unmarshal([]byte(e.HTTPGet(base+"/api/artifacts?"+query.Encode()).Body), &listing); err != nil {
			t.Fatal(err)
		}
	}
	file := func(base, path string) support.HTTPResult {
		t.Helper()
		for index, entry := range listing.Entries {
			if entry.Path == path {
				q := url.Values{"entry": {strconv.Itoa(index)}}
				for key, value := range query {
					q[key] = value
				}
				return e.HTTPGet(base + "/api/artifact-file?" + q.Encode())
			}
		}
		t.Fatalf("listing has no %s: %+v", path, listing.Entries)
		return support.HTTPResult{}
	}
	work := filepath.Join(e.Root, "work.txt")
	read(base)
	if listing.RootsNote != "" || file(base, beside).Body != "out\n" || file(base, work).Body != "work\n" {
		t.Fatalf("trusted project: note %q, previewable %v", listing.RootsNote, listing.Previewable)
	}

	projectDir := filepath.Join(e.Base, "projects", "tr")
	if err := os.Chmod(projectDir, 0o775); err != nil {
		t.Fatal(err)
	}
	read(base)
	if !strings.Contains(listing.RootsNote, projectDir+" is writable by group or others") {
		t.Fatalf("untrusted project note = %q", listing.RootsNote)
	}
	for _, path := range []string{beside, work} {
		if r := file(base, path); r.Status != 403 {
			t.Fatalf("untrusted project served %s: (%d, %s)", path, r.Status, r.Body)
		}
	}
	untrustedWD := url.Values{"entry": {"wd"}, "child": {"work.txt"}}
	for key, value := range query {
		untrustedWD[key] = value
	}
	if r := e.HTTPGet(base + "/api/artifact-file?" + untrustedWD.Encode()); r.Status != 403 {
		t.Fatalf("untrusted project served its working directory: (%d, %s)", r.Status, r.Body)
	}
	withRoot := e.StartWeb("--artifact-root", filepath.Join(e.Root, "outputs"))
	read(withRoot)
	if r := file(withRoot, beside); r.Status != 200 || r.Body != "out\n" {
		t.Fatalf("untrusted project under --artifact-root = (%d, %s)", r.Status, r.Body)
	}
	if r := file(withRoot, work); r.Status != 403 {
		t.Fatalf("untrusted working directory served with --artifact-root elsewhere: (%d, %s)", r.Status, r.Body)
	}
}
