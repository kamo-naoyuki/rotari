package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesJobInfoToolOverMCP(t *testing.T) {
	baseDir, project, runID, jobID := writeJobFixture(t)
	ctx := context.Background()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	server := NewServer()
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
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "rotari_get_job_info" {
		t.Fatalf("tools = %#v, want rotari_get_job_info", tools.Tools)
	}

	result, err := clientSession.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "rotari_get_job_info",
		Arguments: map[string]any{
			"basedir": baseDir,
			"project": project,
			"run_id":  runID,
			"job_id":  jobID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %#v", result.Content)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output GetJobInfoOutput
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatal(err)
	}
	if output.Project != project || output.RunID != runID || output.JobID != jobID {
		t.Fatalf("output identity = %#v", output)
	}
	for _, want := range []string{"job report", "Python exception", "ValueError: bad value", "[REDACTED_PATH]", "recent-log-line"} {
		if !strings.Contains(output.Report, want) {
			t.Errorf("report does not contain %q:\n%s", want, output.Report)
		}
	}
	if strings.Contains(output.Report, baseDir) || strings.Contains(output.Report, "/work/private") {
		t.Fatalf("report contains an unredacted path:\n%s", output.Report)
	}
}

func TestGetJobInfoRejectsUnsafeIdentifiers(t *testing.T) {
	_, _, err := getJobInfo(context.Background(), nil, GetJobInfoInput{
		BaseDir: t.TempDir(), Project: "../project", RunID: "run-1", JobID: "job-1",
	})
	if err == nil {
		t.Fatal("unsafe project name was accepted")
	}
}

func writeJobFixture(t *testing.T) (string, string, string, string) {
	t.Helper()
	baseDir := t.TempDir()
	project, runID, jobID := "demo", "run-1", "job-1"
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
	if err := os.WriteFile(filepath.Join(runDir, jobID, state.StdoutFileName), []byte("recent-log-line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return baseDir, project, runID, jobID
}
