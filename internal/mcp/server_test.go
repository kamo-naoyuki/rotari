package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesJobInfoToolOverMCP(t *testing.T) {
	masterDir := t.TempDir()
	// Two basedirs hold a project with the same name; the run ID alone must
	// select the right one.
	firstBaseDir := t.TempDir()
	secondBaseDir := t.TempDir()
	writeRunFixture(t, masterDir, firstBaseDir, "run-1", "first-log-line")
	writeRunFixture(t, masterDir, secondBaseDir, "run-2", "second-log-line")
	ctx := context.Background()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	server := NewServer(masterDir, Options{})
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(encoded, &schema); err != nil {
			t.Fatal(err)
		}
		// A project is named by basedir_ref and its name, never by a path.
		for property := range schema.Properties {
			if property != "basedir_ref" && (strings.Contains(property, "dir") || strings.Contains(property, "path")) {
				t.Errorf("%s takes a path, %q: %s", tool.Name, property, encoded)
			}
		}
	}
	sort.Strings(names)
	if want := []string{"rotari_cancel", "rotari_check_project", "rotari_compare_runs", "rotari_export_run", "rotari_get_job_info", "rotari_import", "rotari_list_projects", "rotari_preview_import", "rotari_preview_job_control", "rotari_preview_reset", "rotari_preview_run", "rotari_reset", "rotari_resume", "rotari_run_summary", "rotari_start_run", "rotari_suspend", "rotari_wait_run"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("tools = %q, want %q", names, want)
	}

	for _, test := range []struct {
		runID, baseDir, want, other string
	}{
		{"run-1", firstBaseDir, "first-log-line", "second-log-line"},
		{"run-2", secondBaseDir, "second-log-line", "first-log-line"},
	} {
		output := callJobInfo(ctx, t, clientSession, test.runID, "job-1")
		if output.Project != "demo" || output.RunID != test.runID || output.JobID != "job-1" {
			t.Fatalf("%s: output identity = %#v", test.runID, output)
		}
		for _, want := range []string{"job report", "Python exception", "ValueError: bad value", "[REDACTED_PATH]", test.want} {
			if !strings.Contains(output.Report, want) {
				t.Errorf("%s: report does not contain %q:\n%s", test.runID, want, output.Report)
			}
		}
		if strings.Contains(output.Report, test.other) {
			t.Errorf("%s: report contains the other basedir's log %q", test.runID, test.other)
		}
		if strings.Contains(output.Report, test.baseDir) || strings.Contains(output.Report, "/work/private") {
			t.Fatalf("%s: report contains an unredacted path:\n%s", test.runID, output.Report)
		}
	}
}

func TestGetJobInfoRejectsUnknownAndUnsafeIdentifiers(t *testing.T) {
	masterDir := t.TempDir()
	writeRunFixture(t, masterDir, t.TempDir(), "run-1", "log-line")
	// A run in another master directory is not reachable.
	writeRunFixture(t, t.TempDir(), t.TempDir(), "run-elsewhere", "log-line")
	for _, test := range []struct {
		name  string
		input GetJobInfoInput
		want  string
	}{
		{"unregistered run", GetJobInfoInput{RunID: "run-elsewhere", JobID: "job-1"}, `run "run-elsewhere" is not registered`},
		{"slash in run", GetJobInfoInput{RunID: "../run-1", JobID: "job-1"}, "invalid run id"},
		{"backslash in run", GetJobInfoInput{RunID: `run\1`, JobID: "job-1"}, "invalid run id"},
		{"empty run", GetJobInfoInput{JobID: "job-1"}, "invalid run id"},
		{"slash in job", GetJobInfoInput{RunID: "run-1", JobID: "../job-1"}, `job "../job-1" not found`},
		{"backslash in job", GetJobInfoInput{RunID: "run-1", JobID: `job\1`}, `job "job\\1" not found`},
		{"unknown job", GetJobInfoInput{RunID: "run-1", JobID: "job-2"}, `job "job-2" not found`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := getJobInfo(masterDir, test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("getJobInfo(%+v) error = %v, want %q", test.input, err, test.want)
			}
		})
	}
}

// callJobInfo calls rotari_get_job_info through session and decodes its
// structured result.
func callJobInfo(ctx context.Context, t *testing.T, session *mcpsdk.ClientSession, runID, jobID string) GetJobInfoOutput {
	t.Helper()
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "rotari_get_job_info",
		Arguments: map[string]any{"run_id": runID, "job_id": jobID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("%s: tool returned error: %#v", runID, result.Content)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output GetJobInfoOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	return output
}

// writeRunFixture writes a failed run with job "job-1" of project "demo" under
// baseDir and registers it in the run registry under masterDir.
func writeRunFixture(t *testing.T, masterDir, baseDir, runID, logLine string) {
	t.Helper()
	project, jobID := "demo", "job-1"
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{
		ID: jobID, Name: "train", Command: []string{"python", "train.py"}, WorkingDirectory: "/work/private",
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{
		RunID: runID, Status: "failed", ExitCode: 1,
		Results: []model.JobResult{{ID: jobID, ExitCode: 1, Error: "exit status 1", Diagnoses: []model.RuleDiagnosis{{Name: "Python exception", Evidence: "ValueError: bad value", Suggestion: "Inspect the traceback"}}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{CWD: "/work/private", Hostname: "worker.local"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runDir, jobID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, jobID, state.StdoutFileName), []byte(logLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runregistry.Open(masterDir).Register(runregistry.Location{BaseDir: baseDir, ProjectName: project, RunID: runID}); err != nil {
		t.Fatal(err)
	}
}

// TestRunSummaryFollowsAStartingRunAndHidesPaths reads, through the server,
// a run that has started but not written its jobs, which must be running
// with no jobs yet, and an inactive run without jobs, whose error must name
// no state directory.
func TestRunSummaryFollowsAStartingRunAndHidesPaths(t *testing.T) {
	masterDir := t.TempDir()
	baseDir, _ := filepath.Abs(t.TempDir())
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]any{
		paths.QueueFile: model.Queue{},
		paths.LockFile:  model.LockInfo{PID: os.Getpid(), RunID: "starting", Host: host},
		paths.MetaFile:  model.Meta{Phase: "running", LastRunID: "starting"},
	} {
		if err := state.WriteJSON(path, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, runID := range []string{"starting", "abandoned"} {
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := runregistry.Open(masterDir).Register(runregistry.Location{BaseDir: baseDir, ProjectName: "demo", RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := NewServer(masterDir, Options{}).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "1.0.0"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(runID string) *mcpsdk.CallToolResult {
		result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "rotari_run_summary", Arguments: map[string]any{"run_id": runID}})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	result := call("starting")
	encoded, _ := json.Marshal(result.StructuredContent)
	var output RunSummaryOutput
	if err := json.Unmarshal(encoded, &output); err != nil || result.IsError || output.State != "running" || output.Summary.Run.ID != "starting" || output.Summary.Counts.Jobs != 0 {
		t.Fatalf("summary of a starting run = %s (error %v)", encoded, result.IsError)
	}

	result = call("abandoned")
	text, _ := json.Marshal(result.Content)
	if !result.IsError || strings.Contains(string(text), baseDir) || strings.Contains(string(text), masterDir) || !strings.Contains(string(text), "BASEDIR") {
		t.Fatalf("summary of an abandoned run = %s, want an error naming no state directory", text)
	}
}
