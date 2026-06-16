// Package mcp bridges the agent's tools to the Model Context Protocol. NewServer
// exposes any qwen.ToolBox over MCP (reusing the existing tool implementations);
// ToolBox is an MCP-client-backed qwen.ToolBox the classifier consumes at runtime.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lemonishi/autocierge/internal/qwen"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const serverName = "autocierge-tools"
const serverVersion = "1.0.0"

// NewServer builds an MCP server that exposes every tool in tb. Each tool's JSON
// schema is published verbatim (raw schema) so MCP clients see the exact same
// definitions the in-process ToolBox provides; calls delegate to tb.Invoke.
func NewServer(tb qwen.ToolBox) *server.MCPServer {
	// listChanged=false — the tool set is fixed at construction time.
	s := server.NewMCPServer(serverName, serverVersion, server.WithToolCapabilities(false))

	handler := func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		argsJSON, err := json.Marshal(req.GetArguments())
		if err != nil {
			return mcpgo.NewToolResultError("bad arguments: " + err.Error()), nil
		}
		result, ierr := tb.Invoke(ctx, req.Params.Name, string(argsJSON))
		if ierr != nil {
			return mcpgo.NewToolResultError(ierr.Error()), nil
		}
		return mcpgo.NewToolResultText(result), nil
	}

	for _, def := range tb.Definitions() {
		schema, err := json.Marshal(def.Parameters)
		if err != nil {
			panic(fmt.Sprintf("mcp: tool %q: parameters not JSON-serialisable: %v", def.Name, err))
		}
		tool := mcpgo.NewToolWithRawSchema(def.Name, def.Description, schema)
		s.AddTool(tool, handler)
	}
	return s
}
