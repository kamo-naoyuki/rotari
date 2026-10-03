package main

import (
	"context"
	"flag"
	"os"

	rotarimcp "github.com/kamo-naoyuki/rotari/internal/mcp"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// cmdMCP serves rotari's MCP tools over stdio for one master directory.
// Stdout carries the protocol, so nothing else is printed there.
func cmdMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	masterdir := cliString(fs, "masterdir", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("mcp"))
		return 1
	}
	masterDir, err := state.ResolveMasterDir(*masterdir)
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	options := rotarimcp.Options{NewJobID: makeJobID, StartRun: startRunForMCP}
	if err := rotarimcp.NewServer(masterDir, options).Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		printErrorf("MCP server failed: %v", err)
		return 1
	}
	return 0
}

// startRunForMCP starts paths' supervisor, as `rotari run` does, and sends
// it request.
func startRunForMCP(paths state.ProjectPaths, request serverinternal.Request) (serverinternal.Response, error) {
	client, err := startSupervisor(paths)
	if err != nil {
		return serverinternal.Response{}, err
	}
	defer client.Close()
	return client.Send(request)
}
