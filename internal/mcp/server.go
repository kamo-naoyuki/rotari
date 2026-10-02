// Package mcp exposes rotari state through the Model Context Protocol.
package mcp

import (
	"context"

	"github.com/kamo-naoyuki/rotari/internal/report"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetJobInfoInput struct {
	RunID string `json:"run_id" jsonschema:"exact run ID containing the job"`
	JobID string `json:"job_id" jsonschema:"exact job ID to inspect"`
}

type GetJobInfoOutput struct {
	Project string `json:"project"`
	RunID   string `json:"run_id"`
	JobID   string `json:"job_id"`
	Report  string `json:"report"`
}

// NewServer creates an MCP server exposing read-only rotari job inspection.
// It serves only runs registered in the run registry under masterDir.
func NewServer(masterDir string) *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "rotari",
		Version: "0.1.0",
	}, &mcpsdk.ServerOptions{
		Instructions: "Inspect rotari runs read-only. Start with rotari_list_projects to find recent runs and their results, " +
			"then rotari_run_summary for a run's failures grouped by cause, rotari_get_job_info for one job's evidence, " +
			"and rotari_compare_runs to see what a later run fixed. Runs are named by run ID; the server finds their state directory " +
			"and project itself and returns no absolute paths. Paths and hostnames in logs and evidence are redacted where detected.",
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "rotari_list_projects",
		Description: "List the projects of every registered rotari state directory with their state, queue size, run count, and last run's ID, status, and failed job count.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ ListProjectsInput) (*mcpsdk.CallToolResult, ListProjectsOutput, error) {
		output, err := listProjects(masterDir)
		return nil, output, err
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "rotari_run_summary",
		Description: "Summarize one run: job counts, and failed and blocked jobs grouped by cause, each with its jobs, exit codes, an example evidence line, the attempt to inspect, and a suggested fix.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input RunSummaryInput) (*mcpsdk.CallToolResult, RunSummaryOutput, error) {
		output, err := runSummary(masterDir, input)
		return nil, output, err
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "rotari_get_job_info",
		Description: "Return a redacted AI-oriented report for one rotari job, including its status, result, diagnosis, and the log lines around the diagnosis evidence.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input GetJobInfoInput) (*mcpsdk.CallToolResult, GetJobInfoOutput, error) {
		output, err := getJobInfo(masterDir, input)
		return nil, output, err
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "rotari_compare_runs",
		Description: "Compare two runs of one project: which jobs were fixed, still fail, or newly fail, each run's failure cause and whether it changed, and which job definitions changed.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input CompareRunsInput) (*mcpsdk.CallToolResult, CompareRunsOutput, error) {
		output, err := compareRuns(masterDir, input)
		return nil, output, err
	})
	return server
}

// getJobInfo builds the redacted report for one explicitly selected job of a
// run registered under masterDir.
func getJobInfo(masterDir string, input GetJobInfoInput) (GetJobInfoOutput, error) {
	location, err := resolve.RegisteredRun(masterDir, input.RunID)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	paths, err := state.ResolveProjectPaths(location.BaseDir, location.ProjectName)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	jobReport, err := report.Build(store, paths, location.RunID, input.JobID, false, "", true)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	return GetJobInfoOutput{
		Project: location.ProjectName,
		RunID:   location.RunID,
		JobID:   input.JobID,
		Report:  jobReport,
	}, nil
}
