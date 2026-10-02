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
		Instructions: "Inspect rotari jobs by run ID and job ID; the server finds the run's state directory and project itself. The report redacts paths and hostnames where detected.",
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "rotari_get_job_info",
		Description: "Return a redacted AI-oriented report for one rotari job, including its status, result, diagnosis, and bounded recent log output.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, input GetJobInfoInput) (*mcpsdk.CallToolResult, GetJobInfoOutput, error) {
		output, err := getJobInfo(masterDir, input)
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
