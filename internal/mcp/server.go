// Package mcp exposes rotari state through the Model Context Protocol.
package mcp

import (
	"context"
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/report"
	"github.com/kamo-naoyuki/rotari/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type GetJobInfoInput struct {
	BaseDir string `json:"basedir" jsonschema:"rotari state directory containing the project"`
	Project string `json:"project" jsonschema:"rotari project name"`
	RunID   string `json:"run_id" jsonschema:"exact run ID containing the job"`
	JobID   string `json:"job_id" jsonschema:"exact job ID to inspect"`
}

type GetJobInfoOutput struct {
	Project string `json:"project"`
	RunID   string `json:"run_id"`
	JobID   string `json:"job_id"`
	Report  string `json:"report"`
}

// NewServer creates an MCP server exposing read-only rotari job inspection.
func NewServer() *mcpsdk.Server {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "rotari",
		Version: "0.1.0",
	}, &mcpsdk.ServerOptions{
		Instructions: "Inspect rotari jobs using the supplied basedir, project, run ID, and job ID. The report redacts paths and hostnames where detected.",
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "rotari_get_job_info",
		Description: "Return a redacted AI-oriented report for one rotari job, including its status, result, diagnosis, and bounded recent log output.",
	}, getJobInfo)
	return server
}

// GetJobInfo returns the redacted report for one explicitly selected job.
// Both the MCP tool and the terminal command use this function so their job
// lookup and report behavior stays identical.
func GetJobInfo(input GetJobInfoInput) (GetJobInfoOutput, error) {
	if input.BaseDir == "" {
		return GetJobInfoOutput{}, fmt.Errorf("basedir is required")
	}
	paths, err := state.ResolveProjectPaths(input.BaseDir, input.Project)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	jobReport, err := report.Build(store, paths, input.RunID, input.JobID, false, "", true)
	if err != nil {
		return GetJobInfoOutput{}, err
	}
	return GetJobInfoOutput{
		Project: input.Project,
		RunID:   input.RunID,
		JobID:   input.JobID,
		Report:  jobReport,
	}, nil
}

// getJobInfo builds a redacted report for the explicitly selected job.
func getJobInfo(_ context.Context, _ *mcpsdk.CallToolRequest, input GetJobInfoInput) (*mcpsdk.CallToolResult, GetJobInfoOutput, error) {
	output, err := GetJobInfo(input)
	return nil, output, err
}
