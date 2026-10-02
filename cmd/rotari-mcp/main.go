// Command rotari-mcp serves read-only rotari job inspection over MCP stdio.
package main

import (
	"context"
	"log"

	rotarimcp "github.com/kamo-naoyuki/rotari/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// main runs the stdio MCP server until the client disconnects.
func main() {
	if err := rotarimcp.NewServer().Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		log.Printf("rotari MCP server failed: %v", err)
	}
}