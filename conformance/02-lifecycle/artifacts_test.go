package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type artifactRecord struct {
	Version    int `json:"version"`
	Candidates []struct {
		Path    string `json:"path"`
		Basis   string `json:"basis"`
		Sources []struct {
			Kind     string `json:"kind"`
			Value    string `json:"value"`
			Rule     string `json:"rule"`
			Key      string `json:"key"`
			Stream   string `json:"stream"`
			File     string `json:"file"`
			Location string `json:"location"`
		} `json:"sources"`
	} `json:"candidates"`
}

func TestStartedAttemptRecordsArtifactCandidates(t *testing.T) {
	covers(t, "RUN-9")
	e := support.NewEnv(t)
	if err := os.MkdirAll(filepath.Join(e.Root, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, "conf", "train.yaml"), []byte("out_dir: results\nlr: 0.1\nplot: ${out_dir}/plot.png\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The shell code operand contains a path but is code, not a reference.
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "artifacts", "--env", "OUTPUT_DIR=out", "--output", "logs/run.log",
		"--", "sh", "-c", `cat "$1" >/dev/null || cat code/unused.csv; exit 3`, "sh", "conf/train.yaml"))
	if r := e.Rotari("run", "-p", "artifacts", "--quiet"); r.Code == 0 {
		t.Fatalf("run of a job that exits 3 succeeded: %s", r)
	}
	summary := readSummary(t, e, "artifacts")
	attemptID, exitCode := summaryResult(t, summary, jobID)
	if exitCode != 3 {
		t.Fatalf("exit code = %d, want the job's own 3", exitCode)
	}
	runDir := filepath.Join(e.Base, "projects", "artifacts", "runs", summary.RunID)
	var context struct {
		CWD string `json:"cwd"`
	}
	data, err := os.ReadFile(filepath.Join(runDir, "context.json"))
	if err != nil || json.Unmarshal(data, &context) != nil || context.CWD == "" {
		t.Fatalf("context.json = %q, err=%v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(runDir, jobID, "attempts", attemptID, "artifacts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record artifactRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Version != 1 {
		t.Fatalf("version = %d", record.Version)
	}
	type found struct{ path, basis, kind, rule, key, stream, file, location string }
	var got []found
	for _, candidate := range record.Candidates {
		for _, source := range candidate.Sources {
			got = append(got, found{candidate.Path, candidate.Basis, source.Kind, source.Rule, source.Key, source.Stream, source.File, source.Location})
		}
	}
	cwd := context.CWD
	config := filepath.Join(cwd, "conf", "train.yaml")
	want := []found{
		{path: config, basis: "working_directory", kind: "argument", rule: "PATH-R3"},
		{path: filepath.Join(cwd, "out"), basis: "working_directory", kind: "environment", rule: "PATH-R5", key: "OUTPUT_DIR"},
		{path: filepath.Join(cwd, "logs", "run.log"), basis: "working_directory", kind: "output", rule: "PATH-D1", stream: "stdout"},
		{path: filepath.Join(cwd, "logs", "run.log"), basis: "working_directory", kind: "output", rule: "PATH-D1", stream: "stderr"},
		{path: filepath.Join(cwd, "results"), basis: "working_directory", kind: "config", rule: "PATH-R5", key: "out_dir", file: config, location: "out_dir"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact sources:\n got %+v\nwant %+v\nrecord: %s", got, want, data)
	}
}
