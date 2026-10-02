// Command rotari-mcp serves the rotari job information tool over MCP stdio.
package main

import (
	"context"
	"log"

	rotarimcp "github.com/kamo-naoyuki/rotari/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := rotarimcp.NewServer().Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		log.Printf("rotari MCP server failed: %v", err)
	}
}
