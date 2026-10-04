package coordination

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestJobNameLookupReportsSnapshotErrors(t *testing.T) {
	covers(t, "STATE-1")
	for _, test := range []struct {
		name, data, want string
	}{
		{name: "malformed", data: `{"commands":`, want: "unexpected end of JSON input"},
		{name: "newer", data: `{"state_version":999,"commands":[]}`, want: "upgrade rotari"},
		{name: "absent name", data: `{"commands":[]}`, want: "not found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			runID, _ := e.FinishedJobRun("p")
			path := filepath.Join(e.Base, "projects", "p", "runs", runID, "commands.json")
			writeFile(t, path, test.data)
			before := snapshotTree(t, filepath.Join(e.Base, "projects", "p"))
			for _, command := range []string{"run", "retry", "copy", "show"} {
				t.Run(command, func(t *testing.T) {
					checkJobNameLookupError(t, e, command, runID, test.want)
				})
			}
			after := snapshotTree(t, filepath.Join(e.Base, "projects", "p"))
			for path, content := range before {
				if after[path] != content {
					t.Errorf("failed lookup changed %s", path)
				}
			}
		})
	}
}

func checkJobNameLookupError(t *testing.T, e *support.Env, command, runID, want string) {
	t.Helper()
	got := e.Rotari(command, "-p", "p", "--run-id", runID, "--job-name", "train")
	output := got.Stdout + got.Stderr
	if got.Code == 0 || !strings.Contains(output, want) {
		t.Fatalf("lookup should report %q: %s", want, got)
	}
	if want != "not found" && strings.Contains(output, "not found") {
		t.Fatalf("snapshot error hidden as missing job: %s", got)
	}
}
