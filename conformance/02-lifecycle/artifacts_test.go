package lifecycle

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type artifactSource struct {
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	Rule     string `json:"rule"`
	Key      string `json:"key"`
	Stream   string `json:"stream"`
	File     string `json:"file"`
	Location string `json:"location"`
}

type artifactRecord struct {
	Version    int `json:"version"`
	Candidates []struct {
		Path    string           `json:"path"`
		Basis   string           `json:"basis"`
		Sources []artifactSource `json:"sources"`
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
	// The shell code operand is never one value; it is parsed as shell
	// source, where "$1" has no fixed value and code/unused.csv is literal.
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
	cwd := runCWD(t, runDir)
	record, data := readArtifactRecord(t, runDir, jobID, attemptID)
	if record.Version != 6 {
		t.Fatalf("version = %d, want 6", record.Version)
	}
	type found struct{ path, basis, kind, rule, key, stream, file, location string }
	var got []found
	for _, candidate := range record.Candidates {
		for _, source := range candidate.Sources {
			got = append(got, found{candidate.Path, candidate.Basis, source.Kind, source.Rule, source.Key, source.Stream, source.File, source.Location})
		}
	}
	config := filepath.Join(cwd, "conf", "train.yaml")
	want := []found{
		{path: filepath.Join(cwd, "code", "unused.csv"), basis: "working_directory", kind: "shell", rule: "PATH-R3", location: "1:28"},
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

func runCWD(t *testing.T, runDir string) string {
	t.Helper()
	var context struct {
		CWD string `json:"cwd"`
	}
	data, err := os.ReadFile(filepath.Join(runDir, "context.json"))
	if err != nil || json.Unmarshal(data, &context) != nil || context.CWD == "" {
		t.Fatalf("context.json = %q, err=%v", data, err)
	}
	return context.CWD
}

func readArtifactRecord(t *testing.T, runDir, jobID, attemptID string) (artifactRecord, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, jobID, "attempts", attemptID, "artifacts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record artifactRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record, data
}

// artifactExample is one row of "Artifact candidate rules" in
// contracts/02-run-lifecycle-and-execution.md.
type artifactExample struct {
	command  string
	recorded []string
	// rule, when set, is the positive rule that accepted every recorded
	// candidate.
	rule string
}

const artifactExamplesContract = "../../contracts/02-run-lifecycle-and-execution.md"

var (
	exampleFile  = regexp.MustCompile(`^<!-- artifact-example-file: (\S+) -->$`)
	codeSpan     = regexp.MustCompile("`([^`]*)`")
	positiveRule = regexp.MustCompile("^`(PATH-[RD][0-9]+)`$")
)

// readArtifactExamples parses the examples region of the contract: the
// fixture files it declares and every table row with a cell that starts with
// `rotari add`. That cell is the command, the next cell what it records, and
// a first cell naming a PATH-R or PATH-D rule the rule that records it.
func readArtifactExamples(t *testing.T) (map[string]string, []artifactExample) {
	t.Helper()
	data, err := os.ReadFile(artifactExamplesContract)
	if err != nil {
		t.Fatal(err)
	}
	_, region, found := strings.Cut(string(data), "<!-- artifact-examples:start -->")
	region, _, closed := strings.Cut(region, "<!-- artifact-examples:end -->")
	if !found || !closed {
		t.Fatal("contract has no artifact-examples region")
	}
	files := map[string]string{}
	var examples []artifactExample
	lines := strings.Split(region, "\n")
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if match := exampleFile.FindStringSubmatch(line); match != nil {
			if index+1 >= len(lines) || !strings.HasPrefix(lines[index+1], "```") {
				t.Fatalf("example file %s is not followed by a code block", match[1])
			}
			var content []string
			for index += 2; index < len(lines) && !strings.HasPrefix(lines[index], "```"); index++ {
				content = append(content, lines[index])
			}
			files[match[1]] = strings.Join(content, "\n") + "\n"
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		for index := range cells {
			// GitHub Markdown writes a | inside a table cell as \|.
			cells[index] = strings.ReplaceAll(cells[index], `\|`, "|")
		}
		command := slices.IndexFunc(cells, func(cell string) bool { return strings.HasPrefix(cell, "`rotari add ") })
		if command < 0 {
			continue
		}
		if command+1 >= len(cells) {
			t.Fatalf("example row has no Recorded cell after its command: %s", line)
		}
		example := artifactExample{command: codeSpan.FindStringSubmatch(cells[command])[1]}
		for _, match := range codeSpan.FindAllStringSubmatch(cells[command+1], -1) {
			example.recorded = append(example.recorded, match[1])
		}
		if len(example.recorded) == 0 && strings.TrimSpace(cells[command+1]) != "nothing" {
			t.Fatalf("example row's Recorded cell is neither code spans nor nothing: %s", line)
		}
		if match := positiveRule.FindStringSubmatch(cells[0]); match != nil {
			example.rule = match[1]
		}
		examples = append(examples, example)
	}
	if len(examples) == 0 {
		t.Fatal("no artifact examples found")
	}
	return files, examples
}

// shellWords splits a command line the way the user's shell would.
func shellWords(t *testing.T, dir, line string) []string {
	t.Helper()
	command := exec.Command("sh", "-c", "set -- "+line+`; printf '%s\000' "$@"`)
	command.Dir = dir
	output, err := command.Output()
	if err != nil {
		t.Fatalf("sh could not split %q: %v", line, err)
	}
	return strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
}

// TestArtifactCandidateExamples runs every row of the contract's artifact
// candidate examples as written and compares the recorded candidates.
func TestArtifactCandidateExamples(t *testing.T) {
	covers(t, "RUN-9")
	e := support.NewEnv(t)
	files, examples := readArtifactExamples(t)
	for name, content := range files {
		path := filepath.Join(e.Root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const project = "artifact-examples"
	jobIDs := make([]string, len(examples))
	for index, example := range examples {
		words := shellWords(t, e.Root, example.command)
		if len(words) < 2 || words[0] != "rotari" || words[1] != "add" {
			t.Fatalf("example %q does not start with rotari add", example.command)
		}
		args := append([]string{"add", "-p", project}, words[2:]...)
		jobIDs[index] = support.AddedJobID(t, e.MustRotari(args...))
	}
	e.Rotari("run", "-p", project, "--quiet")
	summary := readSummary(t, e, project)
	runDir := filepath.Join(e.Base, "projects", project, "runs", summary.RunID)
	cwd := runCWD(t, runDir)
	for index, example := range examples {
		t.Run(example.command, func(t *testing.T) {
			attemptID, _ := summaryResult(t, summary, jobIDs[index])
			record, data := readArtifactRecord(t, runDir, jobIDs[index], attemptID)
			var got []string
			for _, candidate := range record.Candidates {
				path := candidate.Path
				if candidate.Basis == "working_directory" {
					if relative, err := filepath.Rel(cwd, path); err == nil {
						path = relative
					}
				}
				got = append(got, path)
			}
			if !slices.Equal(got, example.recorded) {
				t.Fatalf("recorded %q, contract says %q\nrecord: %s", got, example.recorded, data)
			}
			if example.rule == "" {
				return
			}
			for _, candidate := range record.Candidates {
				if !slices.ContainsFunc(candidate.Sources, func(source artifactSource) bool { return source.Rule == example.rule }) {
					t.Fatalf("%s was not recorded by %s\nrecord: %s", candidate.Path, example.rule, data)
				}
			}
		})
	}
}

// TestShellVariablesDifferPerArrayTask checks PATH-E1 through the binary:
// each array task records the path its own ROTARI_ARRAY_TASK_ID names, and
// ROTARI_JOB_DIR expands to the attempt's own directory.
func TestShellVariablesDifferPerArrayTask(t *testing.T) {
	covers(t, "RUN-9")
	e := support.NewEnv(t)
	const project = "artifact-array"
	e.MustRotari("add", "-p", project, "--array", "0-1", "--", "bash", "-c", `true > "out/$ROTARI_ARRAY_TASK_ID.log"; true > "$ROTARI_JOB_DIR/result.txt"`)
	e.Rotari("run", "-p", project, "--quiet")
	summary := readSummary(t, e, project)
	runDir := filepath.Join(e.Base, "projects", project, "runs", summary.RunID)
	cwd := runCWD(t, runDir)
	if len(summary.Results) != 2 {
		t.Fatalf("results = %+v, want two array tasks", summary.Results)
	}
	var logs []string
	for _, result := range summary.Results {
		record, data := readArtifactRecord(t, runDir, result.ID, result.AttemptID)
		if len(record.Candidates) != 2 {
			t.Fatalf("%s: record = %s, want two candidates", result.ID, data)
		}
		logs = append(logs, record.Candidates[0].Path)
		attemptDir := filepath.Join(runDir, result.ID, "attempts", result.AttemptID)
		if got, want := record.Candidates[1].Path, filepath.Join(attemptDir, "result.txt"); got != want {
			t.Fatalf("%s: ROTARI_JOB_DIR expanded to %s, want %s", result.ID, got, want)
		}
	}
	slices.Sort(logs)
	if want := []string{filepath.Join(cwd, "out", "0.log"), filepath.Join(cwd, "out", "1.log")}; !slices.Equal(logs, want) {
		t.Fatalf("array task logs = %q, want %q", logs, want)
	}
}
