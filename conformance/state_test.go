package conformance

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Contracts STATE-1 to STATE-4: how rotari reads persisted state written by
// other versions, and that reading never rewrites it. See "State load and
// write contracts" in contracts/04-coordination-and-safety.md.

// setStateVersion rewrites the state_version of the JSON file at path,
// removing it when version is 0, and returns the new content.
func setStateVersion(t *testing.T, path string, version int) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if version == 0 {
		delete(fields, "state_version")
	} else {
		fields["state_version"] = version
	}
	data, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
	return string(data)
}

// snapshotTree returns the content of every file under root by path.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestNewerStateVersionIsRejected(t *testing.T) {
	covers(t, "STATE-1")
	// readCommand is a command that reads the file under test.
	type readCommand struct {
		args []string
	}
	readers := func(file, runID, jobID string) []readCommand {
		switch file {
		case "queue.json":
			return []readCommand{
				{args: []string{"show", "-p", "p", "--queue"}},
				{args: []string{"check", "p"}},
				{args: []string{"add", "-p", "p", "--", "true"}},
				{args: []string{"run", "-p", "p"}},
			}
		case "commands.json":
			return []readCommand{
				{args: []string{"show", "-p", "p", "--run-id", runID}},
				{args: []string{"copy", "-p", "p", "--run-id", runID, "--overwrite"}},
				{args: []string{"export", runID}},
			}
		default:
			return []readCommand{
				{args: []string{"show", "-p", "p", "--run-id", runID}},
				{args: []string{"show", "-p", "p", "--run-id", runID, "--job-id", jobID}},
				{args: []string{"jobs", "p"}},
				{args: []string{"copy", "-p", "p", "--run-id", runID, "--overwrite"}},
				{args: []string{"copy", "-p", "p", "--run-id", runID, "--failed", "--overwrite"}},
				{args: []string{"export", runID}},
			}
		}
	}
	for _, file := range []string{"queue.json", "commands.json", "summary.json"} {
		t.Run(file, func(t *testing.T) {
			e := newEnv(t).in(t)
			runID, _ := e.finishedJobRun("p")
			_, ids := runJobIDs(t, e, "p")
			jobID := ""
			for _, id := range ids {
				jobID = id
			}
			e.mustRotari("add", "-p", "p", "--", "true")
			path := filepath.Join(e.base, "projects", "p", "queue.json")
			if file != "queue.json" {
				path = filepath.Join(e.base, "projects", "p", "runs", runID, file)
			}
			written := setStateVersion(t, path, 99)
			for i, command := range readers(file, runID, jobID) {
				t.Run(command.args[0]+"/"+strconv.Itoa(i), func(t *testing.T) {
					got := e.in(t).rotari(command.args...)
					if got.code == 0 || !strings.Contains(got.stderr+got.stdout, "upgrade rotari") {
						t.Errorf("%s read a newer %s without asking to upgrade: %s", strings.Join(command.args, " "), file, got)
					}
				})
			}
			if data, _ := os.ReadFile(path); string(data) != written {
				t.Errorf("a command rewrote the newer %s", file)
			}
		})
	}
}

func TestUnversionedStateIsVersionOne(t *testing.T) {
	covers(t, "STATE-2")
	e := newEnv(t)
	runID, _ := e.finishedJobRun("p")
	runDir := filepath.Join(e.base, "projects", "p", "runs", runID)
	setStateVersion(t, filepath.Join(runDir, "summary.json"), 0)
	setStateVersion(t, filepath.Join(runDir, "commands.json"), 0)
	e.mustRotari("add", "-p", "p", "--", "true")
	setStateVersion(t, filepath.Join(e.base, "projects", "p", "queue.json"), 0)
	e.mustRotari("show", "-p", "p", "--run-id", runID)
	e.mustRotari("show", "-p", "p", "--queue")
	e.mustRotari("copy", "-p", "p", "--run-id", runID, "--append")
	e.mustRotari("run", "-p", "p", "--quiet")
}

func TestReadingHistoryDoesNotRewriteIt(t *testing.T) {
	covers(t, "STATE-3")
	e := newEnv(t)
	first, attemptID := e.finishedJobRun("p")
	second, _ := e.finishedJobRun("p")
	runDir := filepath.Join(e.base, "projects", "p", "runs", first)
	setStateVersion(t, filepath.Join(runDir, "summary.json"), 0)
	before := snapshotTree(t, runDir)
	base := e.startWeb()
	for _, args := range [][]string{
		{"show", "-p", "p", "--run-id", first},
		{"show", attemptID},
		{"show", "-p", "p", "--run-id", first, "--report"},
		{"jobs", "p"},
		{"diff", first, second},
		{"export", first},
		{"wait", first},
		{"copy", "-p", "p", "--run-id", first, "--overwrite"},
		{"diagnose", "--rules", attemptID},
	} {
		e.rotari(args...)
	}
	e.httpGet(base + "/api/state")
	after := snapshotTree(t, runDir)
	for path, content := range before {
		if after[path] != content {
			t.Errorf("reading the run rewrote %s", strings.TrimPrefix(path, runDir))
		}
	}
}

func TestMalformedLoadSamplesAreSkipped(t *testing.T) {
	covers(t, "STATE-4")
	e := newEnv(t)
	runID, _ := e.finishedJobRun("p")
	samples := filepath.Join(e.base, "projects", "p", "runs", runID, "load_samples.jsonl")
	data, err := os.ReadFile(samples)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, samples, "not json\n\n{\"at\":\n"+string(data))
	e.mustRotari("show", "-p", "p", "--run-id", runID)
	base := e.startWeb()
	if got := e.httpGet(base + "/api/state"); got.status != 200 {
		t.Errorf("Web API state with malformed load samples: status %d: %s", got.status, got.body)
	}
}
