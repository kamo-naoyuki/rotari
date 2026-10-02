// Command rotari-mcp serves the rotari job information tool over MCP stdio.
package main

import (
	"context"
	"log"

	rotarimcp "github.com/kamo-naoyuki/rotari/internal/mcp"
	"github.com/kamo-naoyuki/rotari/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	masterDir, err := state.ResolveMasterDir("")
	if err != nil {
		log.Fatalf("rotari MCP server cannot resolve the master directory: %v", err)
	}
	if err := rotarimcp.NewServer(masterDir).Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		log.Fatalf("rotari MCP server failed: %v", err)
	}
}
