# Plan 7 — MCP Tool Layer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the agent's tools (`lookup_customer`, `lookup_similar_tickets`) over the Model Context Protocol and have the classifier consume them through MCP at runtime — a genuine MCP integration, not a parallel artifact — while the in-process path remains a fallback and the classify loop is untouched.

**Architecture:** `qwen.Client` consumes tools only through the `qwen.ToolBox` interface (`Definitions()` + `Invoke()`). We add (1) a standalone `cmd/mcp-server` that serves the existing `tools.Box` over MCP Streamable HTTP — reusing the same implementations (DRY); and (2) an `internal/mcp` package with an MCP-backed `ToolBox` (mcp-go client) that the main server injects when `MCP_SERVER_URL` is set, falling back to the in-process `tools.Box` otherwise. Because the seam is an interface, only the construction of the `ToolBox` changes — the model still does ordinary function-calling.

**Tech Stack:** Go 1.25; `github.com/mark3labs/mcp-go` (MCP server + client, Streamable HTTP + in-process transports); reuses `internal/tools`, `internal/qwen`, `internal/store`, `internal/config`. Tests use mcp-go's in-process client/server transport (no sockets) with a stub `ToolBox`, plus `testify`.

**Design decisions (locked in brainstorming, 2026-06-16):**
- **Real integration via the `ToolBox` seam** — the classifier calls tools over MCP at runtime; the classify loop in `qwen.Client` does not change.
- **Standalone MCP service over Streamable HTTP** (`cmd/mcp-server`), runnable via `make mcp`; its systemd unit is Plan 8's concern.
- **Reuse `tools.Box`** as the single source of tool logic (the MCP server is a thin protocol adapter; `internal/mcp.NewServer` takes a `qwen.ToolBox`).
- **Fidelity:** the tool schemas surfaced via MCP must be semantically identical to `tools.Box.Definitions()`, so the model sees the same tools either way.
- **Resilience / fallback:** if `MCP_SERVER_URL` is unset, or the MCP server is unreachable at startup, the main server logs it and uses the in-process `tools.Box`. A per-call tool error returns an error (the classify loop already feeds it back as `"error: ..."`, never crashing).
- `mark3labs/mcp-go` over the official SDK — mature, low API churn near the deadline.

---

## ⚠️ SDK API note (read before Task 1)

The code in this plan targets `github.com/mark3labs/mcp-go` and uses these symbols: `server.NewMCPServer`, `server.NewStreamableHTTPServer`, `(*server.MCPServer).AddTool`, `mcp.NewToolWithRawSchema`, `mcp.NewToolResultText`, `mcp.NewToolResultError`, `mcp.CallToolRequest`, `mcp.ListToolsRequest`, `client.NewInProcessClient`, `client.NewStreamableHttpClient`, `(*client.Client).Initialize / ListTools / CallTool`. mcp-go is pre-1.0 and may have renamed or restructured some of these in the pinned version. **The logic, decomposition, and tests in this plan are correct regardless of minor API naming.** In Task 1 Step 1 you will pin the version and run `go doc` against the actual packages; if a symbol differs, adapt the calls to the pinned API (same behavior) and keep going. Do not change the package structure or test intent.

---

## File structure

```
internal/mcp/server.go        → NewServer(tb qwen.ToolBox) *server.MCPServer  (new)
internal/mcp/toolbox.go       → MCP-backed qwen.ToolBox + Dial()              (new)
internal/mcp/mcp_test.go      → round-trip + fidelity tests (in-process)      (new)
cmd/mcp-server/main.go        → serves tools.Box over Streamable HTTP         (new)
internal/config/config.go     → MCPServerURL (MCP_SERVER_URL)                 (modify)
internal/config/config_test.go→ MCP_SERVER_URL round-trip                     (modify)
cmd/server/main.go            → inject MCP ToolBox with in-process fallback   (modify)
Makefile                      → `make mcp` target                            (modify)
app.env.example               → document MCP_SERVER_URL                        (modify)
CLAUDE.md                     → document the MCP tool layer                    (modify)
```

---

## Shared types (already exist — do not redefine)

```go
// internal/qwen/client.go
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON schema for the function arguments
}
type ToolBox interface {
	Definitions() []ToolDefinition
	Invoke(ctx context.Context, name, argsJSON string) (string, error)
}
```

`internal/tools.Box` (`tools.New(s *store.Store) *Box`) already implements `qwen.ToolBox` with the two real tools.

---

## Task 1: MCP server that serves a ToolBox

**Files:**
- Create: `internal/mcp/server.go`
- Test: `internal/mcp/mcp_test.go`

- [ ] **Step 1: Add and pin the dependency, and confirm the API.**

Run:
```bash
go get github.com/mark3labs/mcp-go@latest
go mod tidy
go doc github.com/mark3labs/mcp-go/server | head -60
go doc github.com/mark3labs/mcp-go/mcp NewToolWithRawSchema
go doc github.com/mark3labs/mcp-go/client | head -40
```
Read the output and confirm the symbols listed in the "SDK API note" above. If any differ, note the correct names — you will adapt the code in the steps below to match. Expected: the packages exist and expose server/client/tool constructors.

- [ ] **Step 2: Write the failing test** `internal/mcp/mcp_test.go`. This test stands up `NewServer` over a **stub ToolBox**, connects an in-process client, and verifies list-tools fidelity + call-tool dispatch. (It also exercises the Task 2 `NewToolBox`, written next — so after Task 1 the server half compiles and the client half fails to compile until Task 2. To keep Task 1 green on its own, this step writes ONLY the server-side test; the full round-trip assertions are added in Task 2.)

```go
package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lemonishi/autocierge/internal/mcp"
	"github.com/lemonishi/autocierge/internal/qwen"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpclient "github.com/mark3labs/mcp-go/client"
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
		return "", assertUnknown(name)
	}
	var a struct{ Email string `json:"email"` }
	_ = json.Unmarshal([]byte(argsJSON), &a)
	return `{"found":true,"email":"` + a.Email + `"}`, nil
}

func assertUnknown(name string) error { return &unknownToolErr{name} }

type unknownToolErr struct{ name string }

func (e *unknownToolErr) Error() string { return "unknown tool " + e.name }

// inProcessClient builds a started (but NOT initialized) mcp-go in-process client
// over a NewServer(stub). The caller initializes: TestServerListsTools does it
// directly; the ToolBox tests let NewToolBox do it — this avoids a double
// Initialize (which some mcp-go versions reject).
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
```

> If `go doc` (Step 1) shows different constructor/request names (e.g. `Initialize` takes a populated request, or the in-process client constructor differs), adapt these calls accordingly; keep the assertions.

- [ ] **Step 3: Run the test to verify it FAILS** — `go test ./internal/mcp/ -run TestServerListsTools -v` (expect: undefined `mcp.NewServer`, build failure).

- [ ] **Step 4: Implement** `internal/mcp/server.go`:

```go
// Package mcp bridges the agent's tools to the Model Context Protocol. NewServer
// exposes any qwen.ToolBox over MCP (reusing the existing tool implementations);
// ToolBox is an MCP-client-backed qwen.ToolBox the classifier consumes at runtime.
package mcp

import (
	"context"
	"encoding/json"

	"github.com/lemonishi/autocierge/internal/qwen"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const serverName = "autocierge-tools"
const serverVersion = "1.0.0"

// NewServer builds an MCP server that exposes every tool in tb. Each tool's JSON
// schema is published verbatim (raw schema) so MCP clients see the exact same
// definitions the in-process ToolBox provides; calls are delegated to tb.Invoke.
func NewServer(tb qwen.ToolBox) *server.MCPServer {
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
		schema, _ := json.Marshal(def.Parameters)
		tool := mcpgo.NewToolWithRawSchema(def.Name, def.Description, schema)
		s.AddTool(tool, handler)
	}
	return s
}
```

> Adapt to the pinned API if needed: `req.GetArguments()` returns the call's argument map (if the version exposes args as `req.Params.Arguments` of type `map[string]any` or `json.RawMessage`, marshal/convert that instead); `server.WithToolCapabilities` is optional — drop it if absent.

- [ ] **Step 5: Run the test to verify it PASSES** — `go test ./internal/mcp/ -run TestServerListsTools -v` (expect PASS).

- [ ] **Step 6: Commit:**
```bash
git add go.mod go.sum internal/mcp/server.go internal/mcp/mcp_test.go
git commit -m "feat(mcp): MCP server exposing a qwen.ToolBox over mcp-go

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: MCP-backed ToolBox (the runtime client)

**Files:**
- Create: `internal/mcp/toolbox.go`
- Test: `internal/mcp/mcp_test.go` (extend)

- [ ] **Step 1: Add the failing round-trip + fidelity tests** to `internal/mcp/mcp_test.go`:

```go
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
		// Schemas must be semantically identical (key order / []string vs []any differences ignored).
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
```

- [ ] **Step 2: Run to verify it FAILS** — `go test ./internal/mcp/ -run TestToolBox -v` (expect: undefined `mcp.NewToolBox`).

- [ ] **Step 3: Implement** `internal/mcp/toolbox.go`:

```go
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lemonishi/autocierge/internal/qwen"
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

// NewToolBox initializes the MCP session over an already-connected client, caches
// the tool definitions, and returns a ready ToolBox.
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
	return NewToolBox(ctx, c)
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
		return "", errors.New(text)
	}
	return text, nil
}

// schemaToMap reconstructs a tool's JSON-schema parameters as a map. It prefers
// the raw schema when present, else marshals the structured InputSchema.
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

// firstText returns the concatenated text content of an MCP tool result.
func firstText(res *mcpgo.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := mcpgo.AsTextContent(c); ok {
			return tc.Text
		}
	}
	return ""
}
```

> Adapt to the pinned API if needed: the request struct may be constructed differently (e.g. `mcpgo.CallToolRequest{Params: mcpgo.CallToolParams{Name: name, Arguments: args}}`); `mcpgo.AsTextContent` may instead be a type switch on `mcpgo.TextContent`; `RawInputSchema`/`InputSchema` field names may differ. Keep the behavior identical.

- [ ] **Step 4: Run to verify it PASSES** — `go test ./internal/mcp/ -v` (expect all four tests PASS).

- [ ] **Step 5: Run `go vet ./internal/mcp/`** — expect clean.

- [ ] **Step 6: Commit:**
```bash
git add internal/mcp/toolbox.go internal/mcp/mcp_test.go
git commit -m "feat(mcp): MCP-backed qwen.ToolBox (client) with schema fidelity

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: `cmd/mcp-server` binary + Makefile target

**Files:**
- Create: `cmd/mcp-server/main.go`
- Modify: `Makefile`

- [ ] **Step 1: Implement** `cmd/mcp-server/main.go`. It opens the store (reusing the same pattern as `cmd/server`), builds the in-process `tools.Box`, wraps it with `mcp.NewServer`, and serves it over Streamable HTTP. It reads `DATABASE_URL` (via `config.Load`) and `MCP_LISTEN_ADDR` (default `:8090`).

```go
// Command mcp-server serves Autocierge's agent tools (lookup_customer,
// lookup_similar_tickets) over the Model Context Protocol (Streamable HTTP).
// The main server's MCP-backed ToolBox connects to it; run it via `make mcp`.
//
//	MCP_LISTEN_ADDR  listen address (default ":8090"); tools served at /mcp
//	DATABASE_URL     required (tools are store-backed)
package main

import (
	"context"
	"log"
	"os"

	"github.com/lemonishi/autocierge/internal/config"
	"github.com/lemonishi/autocierge/internal/mcp"
	"github.com/lemonishi/autocierge/internal/store"
	"github.com/lemonishi/autocierge/internal/tools"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	s, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()
	if err := s.SeedDemoCustomers(ctx); err != nil {
		log.Printf("seed demo customers: %v", err)
	}

	addr := os.Getenv("MCP_LISTEN_ADDR")
	if addr == "" {
		addr = ":8090"
	}

	srv := mcp.NewServer(tools.New(s))
	httpSrv := mcpserver.NewStreamableHTTPServer(srv)
	log.Printf("MCP tool server listening on %s (path /mcp)", addr)
	if err := httpSrv.Start(addr); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
```

> Adapt to the pinned API if needed: `NewStreamableHTTPServer(...).Start(addr)` may instead be `ServeHTTP`/`Serve(addr)` or accept options for the path. Confirm the served path (default is typically `/mcp`) and use it consistently in the `MCP_SERVER_URL` you set in Task 4.

- [ ] **Step 2: Verify it builds** — `go build ./cmd/mcp-server` (expect: no output). (Unlike `cmd/eval`, the binary name `mcp-server` does not collide with any directory.)

- [ ] **Step 3: Add the Makefile target.** Change the `.PHONY` line to add `mcp`:
```make
.PHONY: dev run test test-db build tidy frontend eval eval-live mcp
```
and append at the end of `Makefile`:
```make
# Run the MCP tool server locally (Streamable HTTP on :8090, tools at /mcp).
# The main server connects to it when MCP_SERVER_URL is set (see app.env.example).
mcp:
	go run ./cmd/mcp-server
```

- [ ] **Step 4: Smoke-test it starts** (requires the local DB from app.env). Run in one shell:
```bash
make mcp
```
Expect: `MCP tool server listening on :8090 (path /mcp)`. Stop it with Ctrl-C. (If `DATABASE_URL` is unset in your shell, run via `make` which loads app.env.)

- [ ] **Step 5: Commit:**
```bash
git add cmd/mcp-server/main.go Makefile
git commit -m "feat(mcp): cmd/mcp-server serving tools over Streamable HTTP + make mcp

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Config + main-server wiring with in-process fallback

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`
- Modify: `cmd/server/main.go`
- Modify: `app.env.example`

- [ ] **Step 1: Write the failing config test.** Add to `internal/config/config_test.go`:

```go
func TestMCPServerURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MCP_SERVER_URL", "http://127.0.0.1:8090/mcp")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.MCPServerURL != "http://127.0.0.1:8090/mcp" {
		t.Errorf("MCPServerURL = %q", c.MCPServerURL)
	}
}

func TestMCPServerURLDefaultsEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("MCP_SERVER_URL", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.MCPServerURL != "" {
		t.Errorf("MCPServerURL = %q, want empty when unset", c.MCPServerURL)
	}
}
```

- [ ] **Step 2: Run to verify it FAILS** — `go test ./internal/config/ -run MCPServerURL -v` (expect: `c.MCPServerURL` undefined).

- [ ] **Step 3: Add the config field.** In `internal/config/config.go`, add to the `Config` struct (near the other optional fields):

```go
	// MCP tool server (optional). When set, the classifier sources its tools
	// over MCP from this URL (e.g. http://127.0.0.1:8090/mcp); otherwise it uses
	// the in-process tool implementations.
	MCPServerURL string
```

and in `Load()`, after the alerting block:

```go
	c.MCPServerURL = os.Getenv("MCP_SERVER_URL")
```

- [ ] **Step 4: Run to verify it PASSES** — `go test ./internal/config/ -run MCPServerURL -v` (expect PASS).

- [ ] **Step 5: Wire the main server with fallback.** In `cmd/server/main.go`, add the `mcp` import:

```go
	"github.com/lemonishi/autocierge/internal/mcp"
```

Then replace the tool attachment block. The current code is:

```go
	var clf domain.Classifier
	if cfg.DashScopeAPIKey != "" {
		clf = qwen.New(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.QwenModel, nil).
			WithTools(tools.New(s))
		if err := s.SeedDemoCustomers(ctx); err != nil {
			log.Printf("seed demo customers: %v", err)
		}
		log.Printf("classifier: Qwen via DashScope (model=%s) with tools", cfg.QwenModel)
	} else {
		clf = classify.NewFake()
		log.Printf("classifier: fake (DASHSCOPE_API_KEY not set)")
	}
```

Replace it with (selects the MCP-backed ToolBox when configured and reachable, else the in-process Box):

```go
	var clf domain.Classifier
	if cfg.DashScopeAPIKey != "" {
		if err := s.SeedDemoCustomers(ctx); err != nil {
			log.Printf("seed demo customers: %v", err)
		}
		var toolBox qwen.ToolBox = tools.New(s)
		source := "in-process"
		if cfg.MCPServerURL != "" {
			if mtb, err := mcp.Dial(ctx, cfg.MCPServerURL); err != nil {
				log.Printf("mcp: dial %s failed (%v); falling back to in-process tools", cfg.MCPServerURL, err)
			} else {
				toolBox = mtb
				source = "MCP " + cfg.MCPServerURL
			}
		}
		clf = qwen.New(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.QwenModel, nil).
			WithTools(toolBox)
		log.Printf("classifier: Qwen via DashScope (model=%s) with tools (%s)", cfg.QwenModel, source)
	} else {
		clf = classify.NewFake()
		log.Printf("classifier: fake (DASHSCOPE_API_KEY not set)")
	}
```

- [ ] **Step 6: Verify build + vet** — `go build ./... && go vet ./...` (expect clean). The fallback log path means the server still starts when `MCP_SERVER_URL` is unset or the MCP server is down.

- [ ] **Step 7: Document the env var.** In `app.env.example`, add after the Slack block:

```
# ---------------------------------------------------------------------------
# MCP tool server (optional). When set, the classifier sources its tools
# (lookup_customer, lookup_similar_tickets) over MCP from this URL instead of
# in-process. Run the server with `make mcp` (listens on :8090, path /mcp).
# ---------------------------------------------------------------------------
MCP_SERVER_URL=
```

- [ ] **Step 8: Commit:**
```bash
git add internal/config/config.go internal/config/config_test.go cmd/server/main.go app.env.example
git commit -m "feat(mcp): MCP_SERVER_URL config + main-server wiring with in-process fallback

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Documentation + full verification

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Document the MCP layer in** `CLAUDE.md`. Add this bullet to the Stack list immediately after the `Tool layer:` bullet:

```markdown
- MCP: `internal/mcp` + `cmd/mcp-server` — the same tool layer exposed over the
  Model Context Protocol (`github.com/mark3labs/mcp-go`, Streamable HTTP). The
  classifier consumes tools over MCP at runtime when `MCP_SERVER_URL` is set
  (`mcp.Dial` → an MCP-backed `qwen.ToolBox`), falling back to the in-process
  `tools.Box` when unset or the server is unreachable — the classify loop is
  unchanged either way (only which `ToolBox` is injected). Run the server with
  `make mcp` (listens on :8090, tools at `/mcp`). Schemas surfaced over MCP are
  semantically identical to the in-process definitions (fidelity test).
```

- [ ] **Step 2: Run the MCP package tests** — `go test ./internal/mcp/ -v` (expect all PASS).

- [ ] **Step 3: Full verification** — run and confirm:
  - `go vet ./...` — clean
  - `go build ./...` — clean
  - `go test ./...` — all packages green (existing + `internal/mcp`); DB tests run if `TEST_DATABASE_URL` is set, else skip — report which.

- [ ] **Step 4: Commit:**
```bash
git add CLAUDE.md
git commit -m "docs: document the MCP tool layer in CLAUDE.md

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Verification checklist

- [ ] `go test ./internal/mcp/` green: server lists tools, ToolBox definitions are schema-identical to the in-process `tools.Box`, Invoke round-trips, tool errors surface as errors.
- [ ] `make mcp` starts the MCP server on :8090 (tools at `/mcp`).
- [ ] With `MCP_SERVER_URL` set and the MCP server running, the main server logs `with tools (MCP …)`; with it unset, logs `with tools (in-process)`; with it set but server down, logs the fallback warning and still starts.
- [ ] The classify loop in `internal/qwen/client.go` is unchanged — only the injected `ToolBox` differs.
- [ ] `go vet ./...`, `go build ./...`, `go test ./...` all clean/green.
- [ ] No secrets added; `mark3labs/mcp-go` pinned in go.mod/go.sum.

## Manual follow-up (Plan 8)
- The `cmd/mcp-server` gets its own systemd unit and runs on localhost behind the main service; `MCP_SERVER_URL=http://127.0.0.1:8090/mcp` is set in the prod env. (Deployment is Plan 8.)
- Demo: run `make mcp` alongside the server and show the classifier invoking tools over MCP (recorded in `classifications.tools_used`), plus the MCP server as a distinct running service.
