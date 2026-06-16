package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lemonishi/autocierge/internal/mcp"
	"github.com/lemonishi/autocierge/internal/qwen"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBox is a deterministic qwen.ToolBox for tests (no DB).
type stubBox struct{}

func (stubBox) Definitions() []qwen.ToolDefinition {
	return []qwen.ToolDefinition{{
		Name:        "lookup_customer",
		Description: "Look up a customer by email.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"email": map[string]any{"type": "string"}},
			"required":   []string{"email"},
		},
	}}
}

func (stubBox) Invoke(_ context.Context, name, argsJSON string) (string, error) {
	if name != "lookup_customer" {
		return "", &unknownToolErr{name}
	}
	var a struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &a)
	return `{"found":true,"email":"` + a.Email + `"}`, nil
}

type unknownToolErr struct{ name string }

func (e *unknownToolErr) Error() string { return "unknown tool " + e.name }

// inProcessClient builds a started (but NOT initialized) mcp-go in-process client
// over a NewServer(stub). The caller initializes (avoids double-Initialize).
func inProcessClient(t *testing.T) *mcpclient.Client {
	t.Helper()
	srv := mcp.NewServer(stubBox{})
	c, err := mcpclient.NewInProcessClient(srv)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.Start(context.Background()))
	return c
}

func TestServerListsTools(t *testing.T) {
	c := inProcessClient(t)
	ctx := context.Background()
	_, err := c.Initialize(ctx, mcpgo.InitializeRequest{})
	require.NoError(t, err)
	res, err := c.ListTools(ctx, mcpgo.ListToolsRequest{})
	require.NoError(t, err)

	names := map[string]bool{}
	for _, tl := range res.Tools {
		names[tl.Name] = true
	}
	assert.True(t, names["lookup_customer"], "server should expose lookup_customer")
}

func TestToolBoxDefinitionsFidelity(t *testing.T) {
	c := inProcessClient(t)
	tb, err := mcp.NewToolBox(context.Background(), c)
	require.NoError(t, err)

	got := tb.Definitions()
	want := stubBox{}.Definitions()
	require.Len(t, got, len(want))

	byName := map[string]qwen.ToolDefinition{}
	for _, d := range got {
		byName[d.Name] = d
	}
	for _, w := range want {
		g, ok := byName[w.Name]
		require.True(t, ok, "missing tool %s over MCP", w.Name)
		assert.Equal(t, w.Description, g.Description)
		// Schemas must be semantically identical (key order / []string vs []any ignored).
		assert.JSONEq(t, mustJSON(w.Parameters), mustJSON(g.Parameters))
	}
}

func TestToolBoxInvokeRoundTrip(t *testing.T) {
	c := inProcessClient(t)
	tb, err := mcp.NewToolBox(context.Background(), c)
	require.NoError(t, err)

	out, err := tb.Invoke(context.Background(), "lookup_customer", `{"email":"a@b.com"}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"found":true,"email":"a@b.com"}`, out)
}

func TestToolBoxInvokeToolError(t *testing.T) {
	c := inProcessClient(t)
	tb, err := mcp.NewToolBox(context.Background(), c)
	require.NoError(t, err)

	_, err = tb.Invoke(context.Background(), "does_not_exist", `{}`)
	require.Error(t, err) // unknown tool surfaces as an error, mirroring the in-process Box
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
