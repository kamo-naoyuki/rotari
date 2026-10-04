package web

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestWebShowsArtifactCandidates(t *testing.T) {
	covers(t, "WEB-5")
	e := support.NewEnv(t)
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "art", "--", "sh", "-c", "echo x > out.txt; exit 3", "sh", "missing.csv"))
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
	var fromWeb any
	if err := json.Unmarshal([]byte(response.Body), &fromWeb); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromWeb, fromCLI) {
		t.Fatalf("Web and CLI listings differ:\nweb %s\ncli %s", response.Body, shown.Jobs[0].Artifacts)
	}
	if !strings.Contains(response.Body, `"display_path":"out.txt","basis":"working_directory","type":"file"`) ||
		!strings.Contains(response.Body, `"display_path":"missing.csv","basis":"working_directory","type":"missing"`) {
		t.Fatalf("listing lacks the expected entries: %s", response.Body)
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
