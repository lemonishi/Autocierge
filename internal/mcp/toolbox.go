package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lemonishi/supportsentinel/internal/qwen"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// ToolBox is a qwen.ToolBox backed by an MCP client: Definitions() are fetched
// from the MCP server's tool list (once, at construction), and Invoke() dispatches
// MCP call-tool requests. This is what makes the classifier consume tools over MCP.
type ToolBox struct {
	client *mcpclient.Client
	defs   []qwen.ToolDefinition
}

var _ qwen.ToolBox = (*ToolBox)(nil)

// NewToolBox initializes the MCP session over an already-connected (Started)
// client, caches the tool definitions, and returns a ready ToolBox.
func NewToolBox(ctx context.Context, c *mcpclient.Client) (*ToolBox, error) {
	if _, err := c.Initialize(ctx, mcpgo.InitializeRequest{}); err != nil {
		return nil, fmt.Errorf("mcp: initialize: %w", err)
	}
	listed, err := c.ListTools(ctx, mcpgo.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("mcp: list tools: %w", err)
	}
	defs := make([]qwen.ToolDefinition, 0, len(listed.Tools))
	for _, tl := range listed.Tools {
		params, perr := schemaToMap(tl)
		if perr != nil {
			return nil, fmt.Errorf("mcp: tool %q schema: %w", tl.Name, perr)
		}
		defs = append(defs, qwen.ToolDefinition{
			Name:        tl.Name,
			Description: tl.Description,
			Parameters:  params,
		})
	}
	return &ToolBox{client: c, defs: defs}, nil
}

// Dial connects to an MCP server over Streamable HTTP and returns a ready ToolBox.
func Dial(ctx context.Context, url string) (*ToolBox, error) {
	c, err := mcpclient.NewStreamableHttpClient(url)
	if err != nil {
		return nil, fmt.Errorf("mcp: dial %s: %w", url, err)
	}
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("mcp: start %s: %w", url, err)
	}
	tb, err := NewToolBox(ctx, c)
	if err != nil {
		_ = c.Close() // Dial owns the connection; don't leak it on init failure.
		return nil, err
	}
	return tb, nil
}

func (t *ToolBox) Definitions() []qwen.ToolDefinition { return t.defs }

func (t *ToolBox) Invoke(ctx context.Context, name, argsJSON string) (string, error) {
	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("mcp: bad arguments for %q: %w", name, err)
		}
	}
	req := mcpgo.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	res, err := t.client.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("mcp: call %q: %w", name, err)
	}
	text := firstText(res)
	if res.IsError {
		return "", fmt.Errorf("mcp: call %q: tool error: %s", name, text)
	}
	return text, nil
}

// schemaToMap reconstructs a tool's JSON-schema parameters as a map. It prefers
// the raw schema when present, else marshals the structured InputSchema.
//
// Note: mcp-go's structured schema always emits "required": [] (never omits it),
// so a tool whose Parameters omit "required" will round-trip with an empty
// "required" array. Our tools all declare "required", so fidelity holds; keep
// this in mind if a future tool has no required fields.
func schemaToMap(tl mcpgo.Tool) (map[string]any, error) {
	var raw []byte
	if len(tl.RawInputSchema) > 0 {
		raw = tl.RawInputSchema
	} else {
		b, err := json.Marshal(tl.InputSchema)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// firstText returns the first text content of an MCP tool result.
func firstText(res *mcpgo.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := mcpgo.AsTextContent(c); ok {
			return tc.Text
		}
	}
	return ""
}
