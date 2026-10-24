package web

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rotari-dev/rotari/conformance/support"
)

// /api/activity lists the runs of every registered basedir that meet the
// requested range, marks a registered basedir that no longer exists, and
// rejects a reversed range instead of clamping it.
func TestActivityListsRunsAcrossRegisteredBasedirs(t *testing.T) {
	covers(t, "WEB-10")
	e := support.NewEnv(t)
	other := filepath.Join(e.Root, "other-state")
	gone := filepath.Join(e.Root, "gone-state")
	e.MustRotari("add", "-p", "alpha", "--", "true")
	e.MustRotari("run", "-p", "alpha", "--quiet")
	e.MustRotari("add", "-b", other, "-p", "beta", "--", "true")
	e.MustRotari("run", "-b", other, "-p", "beta", "--quiet")
	e.MustRotari("add", "-b", gone, "-p", "gamma", "--", "true")
	e.MustRotari("run", "-b", gone, "-p", "gamma", "--quiet")
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	base := e.StartWeb()
	now := time.Now().UTC()
	query := url.Values{"from": {now.Add(-time.Hour).Format(time.RFC3339)}, "to": {now.Add(time.Hour).Format(time.RFC3339)}}
	response := e.HTTPGet(base + "/api/activity?" + query.Encode())
	if response.Status != 200 {
		t.Fatalf("activity: status %d: %s", response.Status, response.Body)
	}
	var activity struct {
		BaseDirs []struct {
			ID      string `json:"id"`
			Path    string `json:"path"`
			Missing bool   `json:"missing"`
		} `json:"basedirs"`
		Runs []struct {
			BaseDirID string `json:"basedir_id"`
			Project   string `json:"project_name"`
			Lifecycle string `json:"lifecycle"`
			CWD       string `json:"cwd"`
		} `json:"runs"`
	}
	if err := json.Unmarshal([]byte(response.Body), &activity); err != nil {
		t.Fatalf("activity is not JSON: %v\n%s", err, response.Body)
	}
	id := func(path string) string {
		absolute, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString([]byte(filepath.Clean(absolute)))
	}
	projects := map[string]string{}
	for _, run := range activity.Runs {
		projects[run.BaseDirID] += run.Project + " "
		if run.CWD == "" || run.Lifecycle != "finished" {
			t.Errorf("run = %+v, want a finished run with its working directory", run)
		}
	}
	if projects[id(e.Base)] != "alpha " || projects[id(other)] != "beta " {
		t.Errorf("runs by basedir = %v, want alpha in the default basedir and beta in the other", projects)
	}
	missing := false
	for _, entry := range activity.BaseDirs {
		if entry.ID == id(gone) {
			missing = entry.Missing
		}
	}
	if !missing {
		t.Errorf("basedirs = %+v, want the removed basedir listed as missing", activity.BaseDirs)
	}
	reversed := url.Values{"from": query["to"], "to": query["from"]}
	if response := e.HTTPGet(base + "/api/activity?" + reversed.Encode()); response.Status != 400 {
		t.Errorf("reversed range: status %d, want 400: %s", response.Status, response.Body)
	}
}
