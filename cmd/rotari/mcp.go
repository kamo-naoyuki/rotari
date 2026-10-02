package main

import (
	"context"
	"flag"
	"os"

	rotarimcp "github.com/kamo-naoyuki/rotari/internal/mcp"
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
	if err := rotarimcp.NewServer(masterDir).Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		printErrorf("MCP server failed: %v", err)
		return 1
	}
	return 0
}
