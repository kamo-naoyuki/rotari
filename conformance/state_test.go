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

func TestWebShowsNewerRunAsUnreadable(t *testing.T) {
	covers(t, "STATE-1")
	for _, file := range []string{"summary.json", "commands.json"} {
		t.Run(file, func(t *testing.T) {
			e := newEnv(t).in(t)
			readable, _ := e.finishedJobRun("p")
			newer, _ := e.finishedJobRun("p")
			setStateVersion(t, filepath.Join(e.base, "projects", "p", "runs", newer, file), 99)
			got := e.httpGet(e.startWeb() + "/api/state")
			if got.status != 200 {
				t.Fatalf("a run from a newer rotari failed the Web state: status %d: %s", got.status, got.body)
			}
			var state struct {
				Projects []struct {
					Runs []struct {
						RunID      string            `json:"run_id"`
						Status     string            `json:"status"`
						Unreadable string            `json:"unreadable"`
						Jobs       []json.RawMessage `json:"jobs"`
					} `json:"runs"`
				} `json:"projects"`
			}
			if err := json.Unmarshal([]byte(got.body), &state); err != nil || len(state.Projects) != 1 {
				t.Fatalf("Web state: %v: %s", err, got.body)
			}
			for _, run := range state.Projects[0].Runs {
				switch run.RunID {
				case newer:
					if run.Status != "unreadable" || !strings.Contains(run.Unreadable, "upgrade rotari") || len(run.Jobs) != 0 {
						t.Errorf("the newer run: status %q, reason %q, %d jobs; want unreadable with the upgrade message", run.Status, run.Unreadable, len(run.Jobs))
					}
				case readable:
					if run.Status != "finished" || len(run.Jobs) != 1 {
						t.Errorf("the readable run: status %q, %d jobs", run.Status, len(run.Jobs))
					}
				}
			}
		})
	}
}
