# SupportSentinel — Plan 3: Tool Layer (DashScope function-calling)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the Qwen classifier **invoke external tools** during classification to disambiguate hard cases — `lookup_customer(email)` (customer tier/status) and `lookup_similar_tickets(query)` (how past tickets were classified) — via DashScope function-calling, recording every tool invocation into `classifications.tools_used`.

**Architecture:** A new `internal/tools` package implements a `qwen.ToolBox` (tool JSON-schema definitions + an `Invoke` dispatcher) backed by the existing `store`. The Qwen `Client` gains optional tools: when a `ToolBox` is attached, `Classify` runs a tool-calling loop — it offers the tool definitions, executes any `tool_calls` the model requests (appending results to the conversation), records them, and finishes when the model returns the final JSON classification. With no `ToolBox` attached, `Classify` behaves exactly as Plan 2 (single-shot). The orchestrator is unchanged: it already persists `Classification.ToolsUsed` into `classifications.tools_used`. `cmd/server` wires `tools.New(store)` into the client and seeds a few demo customers at startup.

**Tech Stack:** Go 1.25, stdlib `net/http`/`encoding/json`, pgx. DashScope OpenAI-compatible `tools`/`tool_calls`. Offline tests via `httptest` + a fake `ToolBox`; one build-tagged live test.

**Spec:** `docs/superpowers/specs/2026-06-05-supportsentinel-design.md` (§3 tool layer, §4). **Builds on:** Plan 1 (store, `customers` table, orchestrator persists `ToolsUsed`), Plan 2 (qwen client). **Module path:** `github.com/lemonishi/supportsentinel`.

**Environment:** Postgres on port 5433 (`TEST_DATABASE_URL` for DB tests). Live tests need `DASHSCOPE_API_KEY` (in gitignored `app.env`). Commit with the repo's existing git config (identity `Lennon <lemoncode8888@gmail.com>`); never override the author email.

---

## File Structure (Plan 3)

```
internal/store/customers.go        → Customer, SimilarTicket, GetCustomer, UpsertCustomer, FindSimilarTickets (new)
internal/store/customers_test.go    → DB tests (new)
internal/qwen/client.go             → tool wire types, ToolDefinition/ToolBox, WithTools, doChatRaw refactor, Classify tool-loop (modify)
internal/qwen/client_test.go        → tool-calling tests + fake ToolBox (modify)
internal/qwen/live_test.go          → + live tool-calling test (modify)
internal/tools/tools.go             → Box implements qwen.ToolBox over the store (new)
internal/tools/tools_test.go        → DB tests for the two tools (new)
cmd/server/main.go                  → wire tools.New(store) + WithTools; seed demo customers (modify)
internal/store/seed.go              → SeedDemoCustomers (new)
CLAUDE.md                           → note tool layer (modify)
```

---

## Task 1: Store — customer + similar-ticket queries

**Files:**
- Create: `internal/store/customers.go`, `internal/store/customers_test.go`

The `customers` table already exists (Plan 1 schema). This adds typed access + a similar-ticket query.

- [ ] **Step 1: Write the failing test**

`internal/store/customers_test.go`:
```go
package store

import (
	"context"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestUpsertAndGetCustomer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, s.UpsertCustomer(ctx, Customer{
		Email: "vip@acme.com", Name: "Acme VIP", Tier: "enterprise", AccountStatus: "active",
	}))
	// Upsert again with changed tier — should update, not duplicate.
	require.NoError(t, s.UpsertCustomer(ctx, Customer{
		Email: "vip@acme.com", Name: "Acme VIP", Tier: "enterprise_plus", AccountStatus: "active",
	}))

	got, err := s.GetCustomer(ctx, "vip@acme.com")
	require.NoError(t, err)
	require.Equal(t, "enterprise_plus", got.Tier)
	require.Equal(t, "Acme VIP", got.Name)
}

func TestGetCustomerNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetCustomer(context.Background(), "nobody@nowhere.com")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestFindSimilarTickets(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Seed a resolved-ish ticket classified as billing with a matching subject.
	tk, err := s.CreateTicketWithEmail(ctx, "http", domain.Email{
		FromAddr: "a@b.com", Subject: "duplicate invoice charge", Body: "charged twice", DedupeKey: "sim-1",
	})
	require.NoError(t, err)
	urg, typ, dep := domain.UrgencyHigh, domain.TypeBilling, domain.DeptBilling
	require.NoError(t, s.Apply(ctx, Transition{
		TicketID: tk.ID, From: domain.StateNew, To: domain.StateRouted, Actor: "system",
		SetUrgency: &urg, SetType: &typ, SetDepartment: &dep,
	}))

	got, err := s.FindSimilarTickets(ctx, "invoice", 5)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(got), 1)
	require.Equal(t, domain.TypeBilling, got[0].Type)
	require.Contains(t, got[0].Subject, "invoice")

	// A query that matches nothing returns an empty slice, not an error.
	none, err := s.FindSimilarTickets(ctx, "zzzznomatch", 5)
	require.NoError(t, err)
	require.Len(t, none, 0)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5433/supportsentinel_test?sslmode=disable' go test ./internal/store/ -run 'Customer|Similar' -v`
Expected: FAIL — undefined `Customer`, `UpsertCustomer`, etc.

- [ ] **Step 3: Implement**

`internal/store/customers.go`:
```go
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/lemonishi/supportsentinel/internal/domain"
)

// Customer is a seeded account record used by the lookup_customer tool.
type Customer struct {
	Email         string
	Name          string
	Tier          string
	AccountStatus string
}

// SimilarTicket is a past ticket surfaced by the lookup_similar_tickets tool.
type SimilarTicket struct {
	Subject string
	Type    domain.TicketType
	Urgency domain.Urgency
}

// UpsertCustomer inserts or updates a customer by email (idempotent).
func (s *Store) UpsertCustomer(ctx context.Context, c Customer) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO customers (email, name, tier, account_status)
		 VALUES ($1,$2,$3,$4)
		 ON CONFLICT (email) DO UPDATE SET
		   name = EXCLUDED.name, tier = EXCLUDED.tier, account_status = EXCLUDED.account_status`,
		c.Email, c.Name, c.Tier, c.AccountStatus)
	return err
}

// GetCustomer returns the customer with the given email, or ErrNotFound.
func (s *Store) GetCustomer(ctx context.Context, email string) (Customer, error) {
	var c Customer
	err := s.pool.QueryRow(ctx,
		`SELECT email, COALESCE(name,''), COALESCE(tier,''), COALESCE(account_status,'')
		 FROM customers WHERE email = $1`, email).
		Scan(&c.Email, &c.Name, &c.Tier, &c.AccountStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Customer{}, ErrNotFound
	}
	if err != nil {
		return Customer{}, err
	}
	return c, nil
}

// FindSimilarTickets returns up to `limit` already-classified tickets whose email
// subject or body matches the query substring (most recent first). Used to show
// the model how comparable past tickets were classified.
func (s *Store) FindSimilarTickets(ctx context.Context, query string, limit int) ([]SimilarTicket, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(e.subject,''), t.type, t.urgency
		 FROM tickets t JOIN emails e ON e.ticket_id = t.id
		 WHERE t.type IS NOT NULL
		   AND (e.subject ILIKE '%' || $1 || '%' OR e.body ILIKE '%' || $1 || '%')
		 ORDER BY t.created_at DESC
		 LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SimilarTicket{}
	for rows.Next() {
		var st SimilarTicket
		if err := rows.Scan(&st.Subject, &st.Type, &st.Urgency); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `TEST_DATABASE_URL='...' go test ./internal/store/ -v`
Expected: PASS (all store tests incl. the new ones).

- [ ] **Step 5: Commit**

```bash
git add internal/store/customers.go internal/store/customers_test.go
git commit -m "feat(store): customer lookup and similar-ticket query for the tool layer"
```

---

## Task 2: Qwen — tool-calling plumbing

**Files:**
- Modify: `internal/qwen/client.go`, `internal/qwen/client_test.go`

Adds the wire types for `tools`/`tool_calls`, the exported `ToolDefinition`/`ToolBox`, a `WithTools` setter, and refactors the retry call to `doChatRaw` (returns the full assistant message). `Classify` is NOT changed yet (Task 3) — it keeps working via the `doChat` wrapper.

- [ ] **Step 1: Add the failing test**

Append to `internal/qwen/client_test.go`:
```go
func toolCallReply(id, name, args string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{
			"role":    "assistant",
			"content": "",
			"tool_calls": []map[string]any{{
				"id": id, "type": "function",
				"function": map[string]any{"name": name, "arguments": args},
			}},
		}}},
	})
	return string(b)
}

func TestDoChatRawSurfacesToolCalls(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Len(t, body.Tools, 1)
		require.Equal(t, "lookup_customer", body.Tools[0].Function.Name)
		w.Write([]byte(toolCallReply("call_1", "lookup_customer", `{"email":"x@y.com"}`)))
	})
	tools := []toolDef{{Type: "function", Function: functionDef{
		Name: "lookup_customer", Description: "look up a customer", Parameters: map[string]any{"type": "object"},
	}}}
	msg, err := c.doChatRaw(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false, tools)
	require.NoError(t, err)
	require.Len(t, msg.ToolCalls, 1)
	require.Equal(t, "lookup_customer", msg.ToolCalls[0].Function.Name)
	require.Equal(t, `{"email":"x@y.com"}`, msg.ToolCalls[0].Function.Arguments)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/qwen/ -run ToolCalls -v`
Expected: FAIL — `doChatRaw`, `toolDef`, `body.Tools` undefined.

- [ ] **Step 3: Implement the plumbing**

In `internal/qwen/client.go`:

(a) Extend the message type and add tool wire types — replace the existing `chatMessage`/`chatRequest`/`chatResponse` type block with:
```go
type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolCallFunction `json:"function"`
}

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type functionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type toolDef struct {
	Type     string      `json:"type"` // "function"
	Function functionDef `json:"function"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Tools          []toolDef       `json:"tools,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Temperature    float64         `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}
```
(Keep the existing `responseFormat` only once — if it was declared elsewhere, do not duplicate it.)

(b) Add the exported tool abstraction + `WithTools` (place after the `Client` type/`New`):
```go
// ToolDefinition is a function-calling tool the model may invoke during Classify.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON schema for the function arguments
}

// ToolBox supplies callable tools to the classifier.
type ToolBox interface {
	Definitions() []ToolDefinition
	Invoke(ctx context.Context, name, argsJSON string) (string, error)
}

// WithTools attaches a ToolBox so Classify can use function-calling. Returns the
// same client for chaining.
func (c *Client) WithTools(tb ToolBox) *Client {
	c.tools = tb
	return c
}

func toToolDefs(defs []ToolDefinition) []toolDef {
	out := make([]toolDef, 0, len(defs))
	for _, d := range defs {
		out = append(out, toolDef{Type: "function", Function: functionDef{
			Name: d.Name, Description: d.Description, Parameters: d.Parameters,
		}})
	}
	return out
}
```
Add a `tools ToolBox` field to the `Client` struct.

(c) Refactor the HTTP call: rename `doChat` to `doChatRaw` returning the assistant message and accepting tools, then add a thin `doChat` wrapper. Replace the existing `doChat` method with:
```go
// doChatRaw POSTs a chat-completion request and returns the first choice's
// assistant message (content + any tool_calls). Retries on 429/5xx/network with
// context-aware exponential backoff; 4xx is non-retryable.
func (c *Client) doChatRaw(ctx context.Context, messages []chatMessage, jsonMode bool, tools []toolDef) (chatMessage, error) {
	reqBody := chatRequest{Model: c.model, Messages: messages, Temperature: 0}
	if jsonMode {
		reqBody.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	if len(tools) > 0 {
		reqBody.Tools = tools
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return chatMessage{}, fmt.Errorf("marshal request: %w", err)
	}

	backoff := c.retryBackoff
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			return chatMessage{}, fmt.Errorf("build dashscope request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts {
				select {
				case <-time.After(backoff):
					backoff *= 2
				case <-ctx.Done():
					return chatMessage{}, fmt.Errorf("dashscope request cancelled: %w", ctx.Err())
				}
			}
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("dashscope status %d: %s", resp.StatusCode, string(body))
			if attempt < maxAttempts {
				select {
				case <-time.After(backoff):
					backoff *= 2
				case <-ctx.Done():
					return chatMessage{}, fmt.Errorf("dashscope request cancelled: %w", ctx.Err())
				}
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return chatMessage{}, fmt.Errorf("dashscope status %d: %s", resp.StatusCode, string(body))
		}

		var cr chatResponse
		if err := json.Unmarshal(body, &cr); err != nil {
			return chatMessage{}, fmt.Errorf("decode response: %w", err)
		}
		if len(cr.Choices) == 0 {
			return chatMessage{}, errors.New("dashscope returned no choices")
		}
		return cr.Choices[0].Message, nil
	}
	return chatMessage{}, fmt.Errorf("dashscope request failed after %d attempts: %w", maxAttempts, lastErr)
}

// doChat returns just the assistant message content (no tools).
func (c *Client) doChat(ctx context.Context, messages []chatMessage, jsonMode bool) (string, error) {
	msg, err := c.doChatRaw(ctx, messages, jsonMode, nil)
	if err != nil {
		return "", err
	}
	return msg.Content, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/qwen/ -v`
Expected: PASS — all existing tests (doChat/Classify/DraftReply) plus `TestDoChatRawSurfacesToolCalls`. `go vet ./internal/qwen/` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/qwen/
git commit -m "feat(qwen): tool-calling wire types, ToolBox, and doChatRaw refactor"
```

---

## Task 3: Qwen — Classify tool-calling loop

**Files:**
- Modify: `internal/qwen/client.go`, `internal/qwen/client_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/qwen/client_test.go`:
```go
// fakeToolBox returns canned customer data and records what was invoked.
type fakeToolBox struct{ invoked []string }

func (f *fakeToolBox) Definitions() []ToolDefinition {
	return []ToolDefinition{{
		Name:        "lookup_customer",
		Description: "Look up a customer by email",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"email": map[string]any{"type": "string"}},
			"required":   []string{"email"},
		},
	}}
}

func (f *fakeToolBox) Invoke(_ context.Context, name, args string) (string, error) {
	f.invoked = append(f.invoked, name)
	return `{"tier":"enterprise","account_status":"active"}`, nil
}

func TestClassifyRunsToolCallThenClassifies(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			// First turn: model asks to call the tool.
			w.Write([]byte(toolCallReply("call_1", "lookup_customer", `{"email":"vip@acme.com"}`)))
			return
		}
		// Second turn: model returns the final classification, having "seen" the tool result.
		w.Write([]byte(chatReply(validClassificationJSON())))
	})
	tb := &fakeToolBox{}
	c.WithTools(tb)

	got, err := c.Classify(context.Background(), domain.Email{Subject: "charged twice", Body: "vip@acme.com double charge"})
	require.NoError(t, err)
	require.Equal(t, domain.TypeBilling, got.Type)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
	require.Equal(t, []string{"lookup_customer"}, tb.invoked)
	require.Contains(t, got.ToolsUsed, "lookup_customer")
}

func TestClassifySendsToolResultBackToModel(t *testing.T) {
	var secondRoles []string
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 2 {
			var body chatRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			for _, m := range body.Messages {
				secondRoles = append(secondRoles, m.Role)
			}
			w.Write([]byte(chatReply(validClassificationJSON())))
			return
		}
		w.Write([]byte(toolCallReply("call_1", "lookup_customer", `{"email":"v@a.com"}`)))
	})
	c.WithTools(&fakeToolBox{})
	_, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.NoError(t, err)
	// The second request must include the assistant tool-call turn and the tool result.
	require.Equal(t, []string{"system", "user", "assistant", "tool"}, secondRoles)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/qwen/ -run 'ToolCallThen|ToolResultBack' -v`
Expected: FAIL — current `Classify` ignores tools (no second behavior / `ToolsUsed` empty).

- [ ] **Step 3: Reimplement `Classify` as a tool-calling loop**

In `internal/qwen/client.go`, add a constant near the other consts:
```go
const maxToolIterations = 5
```
Replace the entire existing `Classify` method with:
```go
// Classify asks Qwen to classify the email. When a ToolBox is attached it runs a
// function-calling loop: it offers the tools, executes any tool_calls the model
// requests (recording them in ToolsUsed), and finishes when the model returns the
// final JSON classification. With no ToolBox it is a single-shot call. On
// malformed/invalid output it re-prompts once; persistent failure returns an
// error so the orchestrator parks the ticket for human review (fail toward a human).
func (c *Client) Classify(ctx context.Context, e domain.Email) (domain.Classification, error) {
	messages := []chatMessage{
		{Role: "system", Content: classifySystemPrompt},
		{Role: "user", Content: fmt.Sprintf("Subject: %s\n\nBody:\n%s", e.Subject, e.Body)},
	}
	var tools []toolDef
	if c.tools != nil {
		tools = toToolDefs(c.tools.Definitions())
	}
	// JSON-mode only when no tools are offered (response_format + tools can conflict;
	// with tools we rely on the prompt + validation + re-prompt instead).
	jsonMode := len(tools) == 0

	toolsUsed := map[string]any{}
	repromptUsed := false

	for iter := 0; iter < maxToolIterations; iter++ {
		msg, err := c.doChatRaw(ctx, messages, jsonMode, tools)
		if err != nil {
			return domain.Classification{}, err
		}

		if len(msg.ToolCalls) > 0 {
			messages = append(messages, msg) // the assistant's tool-call turn
			for _, tc := range msg.ToolCalls {
				result, ierr := c.tools.Invoke(ctx, tc.Function.Name, tc.Function.Arguments)
				if ierr != nil {
					result = fmt.Sprintf("error: %v", ierr)
				}
				toolsUsed[tc.Function.Name] = map[string]any{
					"arguments": tc.Function.Arguments,
					"result":    result,
				}
				messages = append(messages, chatMessage{
					Role: "tool", ToolCallID: tc.ID, Content: result,
				})
			}
			continue
		}

		cl, perr := parseClassification(msg.Content)
		if perr != nil {
			if repromptUsed {
				return domain.Classification{}, fmt.Errorf("invalid classification after re-prompt: %w", perr)
			}
			repromptUsed = true
			messages = append(messages, msg, chatMessage{Role: "user", Content: reclassifyPrompt})
			continue
		}
		cl.Model = c.model
		if len(toolsUsed) > 0 {
			cl.ToolsUsed = toolsUsed
		}
		return cl, nil
	}
	return domain.Classification{}, fmt.Errorf("classification exceeded %d tool iterations", maxToolIterations)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/qwen/ -v`
Expected: PASS — all prior tests (including the no-tools Classify/re-prompt tests, which still work because `jsonMode` stays true and the loop runs once) plus the two new tool-calling tests. `go vet ./internal/qwen/` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/qwen/
git commit -m "feat(qwen): Classify function-calling loop recording tools_used"
```

---

## Task 4: tools package, seed, and server wiring

**Files:**
- Create: `internal/tools/tools.go`, `internal/tools/tools_test.go`, `internal/store/seed.go`
- Modify: `cmd/server/main.go`, `CLAUDE.md`, `internal/qwen/live_test.go`

- [ ] **Step 1: Write the failing tools test**

`internal/tools/tools_test.go`:
```go
package tools

import (
	"context"
	"os"
	"testing"

	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/lemonishi/supportsentinel/internal/store"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	s, err := store.New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(s.Close)
	_, err = s.Pool().Exec(context.Background(),
		`TRUNCATE audit_log, replies, classifications, emails, tickets, customers RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	return s
}

func TestDefinitionsExposesBothTools(t *testing.T) {
	b := New(newStore(t))
	defs := b.Definitions()
	names := []string{}
	for _, d := range defs {
		names = append(names, d.Name)
	}
	require.ElementsMatch(t, []string{"lookup_customer", "lookup_similar_tickets"}, names)
}

func TestInvokeLookupCustomer(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	require.NoError(t, s.UpsertCustomer(ctx, store.Customer{
		Email: "vip@acme.com", Name: "Acme VIP", Tier: "enterprise", AccountStatus: "active",
	}))
	b := New(s)

	out, err := b.Invoke(ctx, "lookup_customer", `{"email":"vip@acme.com"}`)
	require.NoError(t, err)
	require.Contains(t, out, "enterprise")

	// Unknown customer → a found:false result, not an error.
	out, err = b.Invoke(ctx, "lookup_customer", `{"email":"ghost@nowhere.com"}`)
	require.NoError(t, err)
	require.Contains(t, out, "false")
}

func TestInvokeLookupSimilarTickets(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	tk, err := s.CreateTicketWithEmail(ctx, "http", domain.Email{
		FromAddr: "a@b.com", Subject: "refund for invoice", Body: "x", DedupeKey: "t1",
	})
	require.NoError(t, err)
	urg, typ, dep := domain.UrgencyNormal, domain.TypeBilling, domain.DeptBilling
	require.NoError(t, s.Apply(ctx, store.Transition{
		TicketID: tk.ID, From: domain.StateNew, To: domain.StateRouted, Actor: "system",
		SetUrgency: &urg, SetType: &typ, SetDepartment: &dep,
	}))
	b := New(s)

	out, err := b.Invoke(ctx, "lookup_similar_tickets", `{"query":"invoice"}`)
	require.NoError(t, err)
	require.Contains(t, out, "billing")
}

func TestInvokeUnknownTool(t *testing.T) {
	b := New(newStore(t))
	_, err := b.Invoke(context.Background(), "no_such_tool", `{}`)
	require.Error(t, err)
}

func TestInvokeBadArgsJSON(t *testing.T) {
	b := New(newStore(t))
	_, err := b.Invoke(context.Background(), "lookup_customer", `not json`)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `TEST_DATABASE_URL='...' go test ./internal/tools/ -v`
Expected: FAIL — undefined `New`.

- [ ] **Step 3: Implement the tools package**

`internal/tools/tools.go`:
```go
// Package tools implements qwen.ToolBox over the store: the external tools the
// Qwen classifier can invoke during classification to disambiguate hard cases.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lemonishi/supportsentinel/internal/qwen"
	"github.com/lemonishi/supportsentinel/internal/store"
)

// Box is a store-backed qwen.ToolBox.
type Box struct {
	store *store.Store
}

var _ qwen.ToolBox = (*Box)(nil)

func New(s *store.Store) *Box { return &Box{store: s} }

// Definitions returns the function-calling schemas the model may invoke.
func (b *Box) Definitions() []qwen.ToolDefinition {
	return []qwen.ToolDefinition{
		{
			Name:        "lookup_customer",
			Description: "Look up a customer's account tier and status by their email address. Use this to gauge urgency/priority for known customers.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"email": map[string]any{"type": "string", "description": "the customer's email address"},
				},
				"required": []string{"email"},
			},
		},
		{
			Name:        "lookup_similar_tickets",
			Description: "Find how past support tickets matching a keyword were classified (type and urgency). Use this to stay consistent with prior classifications.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "a short keyword to match past ticket subjects/bodies, e.g. 'invoice'"},
				},
				"required": []string{"query"},
			},
		},
	}
}

// Invoke dispatches a tool call and returns a JSON string result.
func (b *Box) Invoke(ctx context.Context, name, argsJSON string) (string, error) {
	switch name {
	case "lookup_customer":
		var args struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("bad arguments for lookup_customer: %w", err)
		}
		cust, err := b.store.GetCustomer(ctx, args.Email)
		if errors.Is(err, store.ErrNotFound) {
			return `{"found":false}`, nil
		}
		if err != nil {
			return "", err
		}
		return jsonString(map[string]any{
			"found": true, "tier": cust.Tier, "account_status": cust.AccountStatus, "name": cust.Name,
		}), nil

	case "lookup_similar_tickets":
		var args struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("bad arguments for lookup_similar_tickets: %w", err)
		}
		sims, err := b.store.FindSimilarTickets(ctx, args.Query, 5)
		if err != nil {
			return "", err
		}
		results := make([]map[string]any, 0, len(sims))
		for _, s := range sims {
			results = append(results, map[string]any{
				"subject": s.Subject, "type": string(s.Type), "urgency": string(s.Urgency),
			})
		}
		return jsonString(map[string]any{"count": len(results), "tickets": results}), nil

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

- [ ] **Step 4: Run tools tests**

Run: `TEST_DATABASE_URL='...' go test ./internal/tools/ -v`
Expected: PASS (all). `go vet ./internal/tools/` clean.

- [ ] **Step 5: Add the demo seed**

`internal/store/seed.go`:
```go
package store

import "context"

// SeedDemoCustomers upserts a small set of demo customers so the lookup_customer
// tool returns meaningful data in demos. Idempotent.
func (s *Store) SeedDemoCustomers(ctx context.Context) error {
	demo := []Customer{
		{Email: "vip@acme.com", Name: "Acme Corp", Tier: "enterprise", AccountStatus: "active"},
		{Email: "smb@widgets.io", Name: "Widgets Inc", Tier: "business", AccountStatus: "active"},
		{Email: "free@gmail.com", Name: "Casual User", Tier: "free", AccountStatus: "active"},
		{Email: "overdue@latepay.com", Name: "LatePay Ltd", Tier: "business", AccountStatus: "past_due"},
	}
	for _, c := range demo {
		if err := s.UpsertCustomer(ctx, c); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 6: Wire into `cmd/server/main.go`**

In the Qwen branch of `cmd/server/main.go`, attach the tools and seed demo customers. Add `"github.com/lemonishi/supportsentinel/internal/tools"` to imports. Change the Qwen construction:
```go
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
(`ctx` already exists in main from the store setup.)

- [ ] **Step 7: Add a live tool-calling test**

Append to `internal/qwen/live_test.go` (under the same `//go:build live` file):
```go
// stubToolBox is a live-test ToolBox returning canned data, to exercise the
// real model's function-calling without a DB.
type stubToolBox struct{ called []string }

func (s *stubToolBox) Definitions() []ToolDefinition {
	return []ToolDefinition{{
		Name:        "lookup_customer",
		Description: "Look up a customer's tier and status by email.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"email": map[string]any{"type": "string"}},
			"required":   []string{"email"},
		},
	}}
}
func (s *stubToolBox) Invoke(_ context.Context, name, args string) (string, error) {
	s.called = append(s.called, name)
	return `{"found":true,"tier":"enterprise","account_status":"active"}`, nil
}

func TestLiveClassifyWithTools(t *testing.T) {
	c := liveClient(t)
	tb := &stubToolBox{}
	c.WithTools(tb)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	got, err := c.Classify(ctx, domain.Email{
		Subject: "billing problem from vip@acme.com",
		Body:    "I think I was overcharged on my enterprise plan. Please check my account vip@acme.com.",
	})
	require.NoError(t, err)
	require.True(t, domain.ValidType(got.Type))
	t.Logf("live tools classification: type=%s urgency=%s tools_used=%v called=%v",
		got.Type, got.Urgency, got.ToolsUsed, tb.called)
}
```

- [ ] **Step 8: Update `CLAUDE.md`**

Under "## Stack", add a bullet:
```markdown
- Tool layer: `internal/tools` (DashScope function-calling) — `lookup_customer` and
  `lookup_similar_tickets`, store-backed, attached via `qwen.Client.WithTools`. The
  classifier invokes them during Classify; invocations are recorded in
  `classifications.tools_used`. Demo customers seeded at server startup.
```

- [ ] **Step 9: Full verification**

Run:
```bash
go vet ./...
go build ./...
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5433/supportsentinel_test?sslmode=disable' go test ./...
```
Expected: build clean; all tests pass (tools + qwen + store etc.; live tests excluded).

- [ ] **Step 10: Commit**

```bash
git add internal/tools/ internal/store/seed.go cmd/server/main.go CLAUDE.md internal/qwen/live_test.go
git commit -m "feat(tools): customer + similar-ticket tools wired into the classifier"
```

---

## Plan 3 Definition of Done

- [ ] `go build ./...` and `go vet ./...` clean.
- [ ] `go test ./...` passes (tools + qwen function-calling tests via httptest; live excluded; DB tests pass).
- [ ] The Qwen `Classify` runs a tool-calling loop when a `ToolBox` is attached and records invocations in `Classification.ToolsUsed`; the orchestrator persists them to `classifications.tools_used`.
- [ ] `cmd/server` seeds demo customers and attaches the tools when `DASHSCOPE_API_KEY` is set.
- [ ] Manual live: a server POST whose email names a seeded customer shows tool invocations recorded in the `classifications.tools_used` column; `go test -tags live ./internal/qwen/ -run LiveClassifyWithTools` logs the tools the real model called.

---

## Roadmap — Subsequent Plans

- **Plan 4 — React dashboard** (queue, detail incl. reasoning/confidence/tools-used, both checkpoint controls, audit timeline; `//go:embed`).
- **Plan 5 — IMAP ingestion + Slack/email alerting.**
- **Plan 6 — Eval harness + gold dataset + threshold calibration.**
- **Plan 7 — Deployment + submission deliverables.**
