# SupportSentinel — Plan 2: Qwen / DashScope Integration

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the fake classifier with a real **Qwen** classifier that calls **Alibaba Cloud Model Studio (DashScope)** over its OpenAI-compatible endpoint, returning structured classifications and drafted replies — with bounded retry, malformed-output recovery, and "fail toward a human" preserved.

**Architecture:** A new `internal/qwen` package implements `domain.Classifier` by POSTing chat completions to DashScope's `compatible-mode/v1/chat/completions`. `Classify` requests JSON-mode output, parses + validates it against the fixed taxonomies (re-prompting once on malformed JSON), and clamps/derives fields; `DraftReply` returns free-text. The low-level call has bounded exponential-backoff retry on 429/5xx/network errors. On exhaustion or persistent malformed output it returns an error — the orchestrator already parks such tickets in `AWAITING_CLASSIFICATION_REVIEW` (fail toward a human). `cmd/server` wires the Qwen client when `DASHSCOPE_API_KEY` is set, otherwise falls back to the fake classifier so local dev works without a key. **`internal/qwen/client.go` is the primary "Proof of Alibaba Cloud" artifact.**

**Tech Stack:** Go 1.25, standard library `net/http` (no SDK — keeps the proof file self-evidently calling DashScope), `encoding/json`. Tests use `httptest` to simulate DashScope (offline, deterministic); one build-tagged live smoke test hits the real endpoint.

**Spec:** `docs/superpowers/specs/2026-06-05-supportsentinel-design.md` (§4 Qwen client, §7 resilience).
**Builds on:** Plan 1 (`domain.Classifier`, orchestrator, fake classifier remain).
**Module path:** `github.com/lemonishi/supportsentinel`.

**Environment facts:**
- Region: **International (Singapore)** → default base URL `https://dashscope-intl.aliyuncs.com/compatible-mode/v1`, overridable via `DASHSCOPE_BASE_URL`.
- Default model: `qwen-max`, overridable via `QWEN_MODEL`.
- The user HAS a `DASHSCOPE_API_KEY` (lives in gitignored `app.env`); the live smoke test can be run manually.
- DB tests still need `TEST_DATABASE_URL` (local Postgres on port 5433). The Qwen contract tests do NOT need a DB or a key.

---

## File Structure (Plan 2)

```
internal/qwen/client.go        → Client, New, doChat (retry), Classify, DraftReply, prompts, parse/validate
internal/qwen/client_test.go    → offline contract tests via httptest (no key, no DB)
internal/qwen/live_test.go      → //go:build live — real-endpoint smoke test (manual)
internal/config/config.go       → + DashScopeAPIKey, DashScopeBaseURL, QwenModel (modify)
internal/config/config_test.go  → + defaults test (modify)
cmd/server/main.go              → wire Qwen when key present, else fake (modify)
app.env.example                 → + DASHSCOPE_BASE_URL, QWEN_MODEL (modify)
CLAUDE.md                       → note Qwen client is live + the proof file (modify)
```

---

## Task 1: Config — DashScope settings

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `app.env.example`

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go`:
```go
func TestLoadDashScopeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DASHSCOPE_API_KEY", "sk-test")
	t.Setenv("DASHSCOPE_BASE_URL", "")
	t.Setenv("QWEN_MODEL", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DashScopeAPIKey != "sk-test" {
		t.Fatalf("DashScopeAPIKey = %q", c.DashScopeAPIKey)
	}
	if c.DashScopeBaseURL != "https://dashscope-intl.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("DashScopeBaseURL default = %q", c.DashScopeBaseURL)
	}
	if c.QwenModel != "qwen-max" {
		t.Fatalf("QwenModel default = %q", c.QwenModel)
	}
}

func TestLoadDashScopeOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DASHSCOPE_BASE_URL", "https://example/v1")
	t.Setenv("QWEN_MODEL", "qwen-plus")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DashScopeBaseURL != "https://example/v1" {
		t.Fatalf("DashScopeBaseURL override = %q", c.DashScopeBaseURL)
	}
	if c.QwenModel != "qwen-plus" {
		t.Fatalf("QwenModel override = %q", c.QwenModel)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run DashScope -v`
Expected: FAIL — `c.DashScopeAPIKey` undefined.

- [ ] **Step 3: Implement**

In `internal/config/config.go`, add fields to `Config`:
```go
	DashScopeAPIKey  string
	DashScopeBaseURL string
	QwenModel        string
```
And in `Load()`, after the existing assignments (before `return c, nil`), add:
```go
	c.DashScopeAPIKey = os.Getenv("DASHSCOPE_API_KEY")
	c.DashScopeBaseURL = getenv("DASHSCOPE_BASE_URL", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1")
	c.QwenModel = getenv("QWEN_MODEL", "qwen-max")
```
(Note: `DASHSCOPE_API_KEY` is intentionally NOT required at load time — `cmd/server` falls back to the fake classifier when it's empty, so local dev works without a key.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests).

- [ ] **Step 5: Update `app.env.example`**

Replace the commented Qwen line:
```bash
# Qwen / DashScope (added in Plan 2)
# DASHSCOPE_API_KEY=
```
with:
```bash
# Qwen / Alibaba Cloud Model Studio (DashScope, OpenAI-compatible endpoint)
# Required for real classification; if unset the server uses the fake classifier.
DASHSCOPE_API_KEY=
# International (Singapore) endpoint by default; switch to
# https://dashscope.aliyuncs.com/compatible-mode/v1 for China (Beijing).
DASHSCOPE_BASE_URL=https://dashscope-intl.aliyuncs.com/compatible-mode/v1
QWEN_MODEL=qwen-max
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/ app.env.example
git commit -m "feat(config): DashScope API key, base URL, and model settings"
```

---

## Task 2: Qwen client — chat plumbing with retry

**Files:**
- Create: `internal/qwen/client.go`
- Test: `internal/qwen/client_test.go`

This task builds the low-level `doChat` call (request/response types + bounded retry) and the `Client` skeleton. `Classify`/`DraftReply` come in Tasks 3–4.

- [ ] **Step 1: Write the failing test**

`internal/qwen/client_test.go`:
```go
package qwen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestClient points a Client at an httptest server with near-zero backoff.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("sk-test", srv.URL, "qwen-max", srv.Client())
	c.retryBackoff = time.Millisecond // keep tests fast
	return c
}

// chatReply is a minimal OpenAI-compatible completion response with the given content.
func chatReply(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"role": "assistant", "content": content}},
		},
	})
	return string(b)
}

func TestDoChatRetriesOn500ThenSucceeds(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(chatReply("hello")))
	})
	out, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.NoError(t, err)
	require.Equal(t, "hello", out)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestDoChatReturnsErrorAfterExhaustingRetries(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	_, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.Error(t, err)
}

func TestDoChatDoesNotRetryOn400(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	})
	_, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.Error(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&calls)) // 4xx is non-retryable
}

func TestDoChatSendsAuthAndModel(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "qwen-max", body.Model)
		require.Len(t, body.Messages, 1)
		w.Write([]byte(chatReply("ok")))
	})
	out, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, false)
	require.NoError(t, err)
	require.Equal(t, "ok", out)
}

func TestDoChatJSONModeSetsResponseFormat(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.NotNil(t, body.ResponseFormat)
		require.Equal(t, "json_object", body.ResponseFormat.Type)
		w.Write([]byte(chatReply("{}")))
	})
	_, err := c.doChat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}, true)
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/qwen/ -v`
Expected: FAIL — `New`, `Client`, `chatMessage`, `chatRequest` undefined.

- [ ] **Step 3: Implement the client plumbing**

`internal/qwen/client.go`:
```go
// Package qwen implements domain.Classifier by calling Alibaba Cloud Model
// Studio (DashScope) over its OpenAI-compatible chat-completions endpoint.
//
// This file is the project's primary "Proof of Alibaba Cloud" artifact: it
// authenticates with an Alibaba Cloud DashScope API key and POSTs to the
// DashScope compatible-mode endpoint.
package qwen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/lemonishi/supportsentinel/internal/domain"
)

const (
	// DefaultBaseURL is the Alibaba Cloud Model Studio (DashScope) International
	// (Singapore) OpenAI-compatible endpoint.
	DefaultBaseURL = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	// DefaultModel is the default Qwen model for classification.
	DefaultModel = "qwen-max"

	maxAttempts = 3
)

// Client is a Qwen classifier backed by Alibaba Cloud DashScope.
type Client struct {
	apiKey       string
	baseURL      string
	model        string
	http         *http.Client
	retryBackoff time.Duration // initial backoff; doubles each retry
}

var _ domain.Classifier = (*Client)(nil)

// New returns a Qwen Client. Empty baseURL/model fall back to the defaults; a
// nil httpClient gets a 30s-timeout client.
func New(apiKey, baseURL, model string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if model == "" {
		model = DefaultModel
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		apiKey:       apiKey,
		baseURL:      baseURL,
		model:        model,
		http:         httpClient,
		retryBackoff: 500 * time.Millisecond,
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Temperature    float64         `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// doChat POSTs a chat-completion request and returns the first choice's content.
// It retries on 429/5xx/network errors with exponential backoff (maxAttempts);
// 4xx responses are returned immediately (non-retryable).
func (c *Client) doChat(ctx context.Context, messages []chatMessage, jsonMode bool) (string, error) {
	reqBody := chatRequest{Model: c.model, Messages: messages, Temperature: 0}
	if jsonMode {
		reqBody.ResponseFormat = &responseFormat{Type: "json_object"}
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	backoff := c.retryBackoff
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if attempt < maxAttempts {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("dashscope status %d: %s", resp.StatusCode, string(body))
			if attempt < maxAttempts {
				time.Sleep(backoff)
				backoff *= 2
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("dashscope status %d: %s", resp.StatusCode, string(body))
		}

		var cr chatResponse
		if err := json.Unmarshal(body, &cr); err != nil {
			return "", fmt.Errorf("decode response: %w", err)
		}
		if len(cr.Choices) == 0 {
			return "", errors.New("dashscope returned no choices")
		}
		return cr.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("dashscope request failed after %d attempts: %w", maxAttempts, lastErr)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/qwen/ -v`
Expected: PASS (5 doChat tests). `go vet ./internal/qwen/` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/qwen/
git commit -m "feat(qwen): DashScope chat client with bounded retry"
```

---

## Task 3: Classify — structured output, validation, re-prompt

**Files:**
- Modify: `internal/qwen/client.go`
- Modify: `internal/qwen/client_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/qwen/client_test.go`:
```go
func validClassificationJSON() string {
	return `{"urgency":"high","type":"billing","department":"billing","confidence":0.91,"reasoning":"double charge"}`
}

func TestClassifyParsesValidResponse(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(validClassificationJSON())))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "charged twice", Body: "help"})
	require.NoError(t, err)
	require.Equal(t, domain.UrgencyHigh, got.Urgency)
	require.Equal(t, domain.TypeBilling, got.Type)
	require.Equal(t, domain.DeptBilling, got.Department)
	require.InEpsilon(t, 0.91, got.Confidence, 0.001)
	require.Equal(t, "qwen-max", got.Model)
}

func TestClassifyDerivesDepartmentWhenInvalid(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(`{"urgency":"normal","type":"technical","department":"nonsense","confidence":0.8}`)))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "bug", Body: "x"})
	require.NoError(t, err)
	require.Equal(t, domain.DeptEngineering, got.Department) // derived from type
}

func TestClassifyClampsConfidence(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(`{"urgency":"low","type":"general","department":"support_tier1","confidence":5}`)))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "hi", Body: "x"})
	require.NoError(t, err)
	require.Equal(t, 1.0, got.Confidence)
}

func TestClassifyStripsCodeFences(t *testing.T) {
	fenced := "```json\n" + validClassificationJSON() + "\n```"
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(fenced)))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.NoError(t, err)
	require.Equal(t, domain.TypeBilling, got.Type)
}

func TestClassifyRepromptsOnMalformedThenSucceeds(t *testing.T) {
	var calls int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Write([]byte(chatReply("not json at all")))
			return
		}
		w.Write([]byte(chatReply(validClassificationJSON())))
	})
	got, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.NoError(t, err)
	require.Equal(t, domain.TypeBilling, got.Type)
	require.Equal(t, int32(2), atomic.LoadInt32(&calls)) // re-prompted once
}

func TestClassifyErrorsOnPersistentMalformed(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply("still not json")))
	})
	_, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.Error(t, err) // orchestrator will park this for human review
}

func TestClassifyErrorsOnInvalidEnumAfterReprompt(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(chatReply(`{"urgency":"WAT","type":"billing","confidence":0.5}`)))
	})
	_, err := c.Classify(context.Background(), domain.Email{Subject: "x", Body: "y"})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/qwen/ -run Classify -v`
Expected: FAIL — `Classify` undefined.

- [ ] **Step 3: Implement Classify + parsing**

Append to `internal/qwen/client.go` (add `"strings"` to the import block):
```go
const classifySystemPrompt = `You are a support-ticket classifier. Classify the customer's email and respond with ONLY a JSON object (no prose, no code fences) with exactly these fields:
{"urgency": one of ["low","normal","high","critical"],
 "type": one of ["billing","technical","account","feature_request","general"],
 "department": one of ["billing","engineering","accounts","product","support_tier1"],
 "confidence": a number between 0 and 1 (your confidence in the classification),
 "reasoning": a one-sentence explanation}
Use "critical" only for outages, data loss, or urgent business impact.`

const reclassifyPrompt = `Your previous response was not valid JSON in the required schema. Respond again with ONLY the JSON object described, nothing else.`

// Classify asks Qwen to classify the email and returns a validated Classification.
// On malformed/invalid output it re-prompts once; if still bad it returns an error
// (the orchestrator parks such tickets for human review — fail toward a human).
func (c *Client) Classify(ctx context.Context, e domain.Email) (domain.Classification, error) {
	messages := []chatMessage{
		{Role: "system", Content: classifySystemPrompt},
		{Role: "user", Content: fmt.Sprintf("Subject: %s\n\nBody:\n%s", e.Subject, e.Body)},
	}

	content, err := c.doChat(ctx, messages, true)
	if err != nil {
		return domain.Classification{}, err
	}
	cl, perr := parseClassification(content)
	if perr != nil {
		// One re-prompt with the schema reminder.
		messages = append(messages,
			chatMessage{Role: "assistant", Content: content},
			chatMessage{Role: "user", Content: reclassifyPrompt},
		)
		content, err = c.doChat(ctx, messages, true)
		if err != nil {
			return domain.Classification{}, err
		}
		cl, perr = parseClassification(content)
		if perr != nil {
			return domain.Classification{}, fmt.Errorf("invalid classification after re-prompt: %w", perr)
		}
	}
	cl.Model = c.model
	return cl, nil
}

// parseClassification parses and validates the model's JSON against the fixed
// taxonomies. Invalid urgency/type is an error (triggers a re-prompt). An
// invalid/empty department is derived from the type. Confidence is clamped to [0,1].
func parseClassification(s string) (domain.Classification, error) {
	var raw struct {
		Urgency    string  `json:"urgency"`
		Type       string  `json:"type"`
		Department string  `json:"department"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(stripCodeFences(s)), &raw); err != nil {
		return domain.Classification{}, fmt.Errorf("unmarshal classification: %w", err)
	}

	urg := domain.Urgency(strings.ToLower(strings.TrimSpace(raw.Urgency)))
	if !domain.ValidUrgency(urg) {
		return domain.Classification{}, fmt.Errorf("invalid urgency %q", raw.Urgency)
	}
	typ := domain.TicketType(strings.ToLower(strings.TrimSpace(raw.Type)))
	if !domain.ValidType(typ) {
		return domain.Classification{}, fmt.Errorf("invalid type %q", raw.Type)
	}
	dep := domain.Department(strings.ToLower(strings.TrimSpace(raw.Department)))
	if !domain.ValidDepartment(dep) {
		dep = domain.DepartmentForType(typ)
	}
	conf := raw.Confidence
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}
	return domain.Classification{
		Urgency:    urg,
		Type:       typ,
		Department: dep,
		Confidence: conf,
		Reasoning:  raw.Reasoning,
	}, nil
}

// stripCodeFences removes a leading ```json / ``` fence and trailing ``` if the
// model wrapped its JSON despite being asked not to.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/qwen/ -v`
Expected: PASS (all). `go vet ./internal/qwen/` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/qwen/
git commit -m "feat(qwen): Classify with JSON-mode output, validation, and one re-prompt"
```

---

## Task 4: DraftReply

**Files:**
- Modify: `internal/qwen/client.go`, `internal/qwen/client_test.go`

- [ ] **Step 1: Add failing test**

Append to `internal/qwen/client_test.go`:
```go
func TestDraftReplyReturnsText(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body chatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Nil(t, body.ResponseFormat) // draft is free text, not JSON mode
		w.Write([]byte(chatReply("Hi, we've refunded the duplicate charge. Sorry for the trouble!")))
	})
	out, err := c.DraftReply(context.Background(),
		domain.Ticket{Urgency: domain.UrgencyHigh, Type: domain.TypeBilling},
		domain.Email{Subject: "charged twice", Body: "double charge"})
	require.NoError(t, err)
	require.Contains(t, out, "refunded")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/qwen/ -run DraftReply -v`
Expected: FAIL — `DraftReply` undefined.

- [ ] **Step 3: Implement**

Append to `internal/qwen/client.go`:
```go
const draftSystemPrompt = `You are a helpful, professional customer-support agent. Write a concise, empathetic reply to the customer's email. Address their issue directly. Do not invent specific facts (order numbers, dates, amounts) that are not in the email. Sign off as "Support".`

// DraftReply asks Qwen to write a customer-facing reply (free text, not JSON).
func (c *Client) DraftReply(ctx context.Context, t domain.Ticket, e domain.Email) (string, error) {
	user := fmt.Sprintf("Ticket urgency: %s\nTicket type: %s\n\nCustomer email:\nSubject: %s\n\n%s",
		t.Urgency, t.Type, e.Subject, e.Body)
	messages := []chatMessage{
		{Role: "system", Content: draftSystemPrompt},
		{Role: "user", Content: user},
	}
	return c.doChat(ctx, messages, false)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/qwen/ -v`
Expected: PASS (all). `go vet ./internal/qwen/` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/qwen/
git commit -m "feat(qwen): DraftReply free-text generation"
```

---

## Task 5: Live smoke test + wire into cmd/server

**Files:**
- Create: `internal/qwen/live_test.go`
- Modify: `cmd/server/main.go`, `CLAUDE.md`

- [ ] **Step 1: Add the build-tagged live smoke test**

`internal/qwen/live_test.go`:
```go
//go:build live

// Live smoke test against the real Alibaba Cloud DashScope endpoint.
// Run manually:  DASHSCOPE_API_KEY=... go test -tags live ./internal/qwen/ -run Live -v
// Uses DASHSCOPE_BASE_URL / QWEN_MODEL if set, else the defaults.
package qwen

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/stretchr/testify/require"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("DASHSCOPE_API_KEY")
	if key == "" {
		t.Skip("DASHSCOPE_API_KEY not set; skipping live test")
	}
	return New(key, os.Getenv("DASHSCOPE_BASE_URL"), os.Getenv("QWEN_MODEL"), nil)
}

func TestLiveClassify(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := c.Classify(ctx, domain.Email{
		Subject: "URGENT: production is completely down",
		Body:    "Our whole site has been returning 500 errors for 20 minutes. We are losing sales.",
	})
	require.NoError(t, err)
	require.True(t, domain.ValidUrgency(got.Urgency))
	require.True(t, domain.ValidType(got.Type))
	t.Logf("live classification: urgency=%s type=%s dept=%s conf=%.2f reasoning=%q",
		got.Urgency, got.Type, got.Department, got.Confidence, got.Reasoning)
}

func TestLiveDraftReply(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := c.DraftReply(ctx,
		domain.Ticket{Urgency: domain.UrgencyHigh, Type: domain.TypeBilling},
		domain.Email{Subject: "double charged", Body: "I was billed twice this month."})
	require.NoError(t, err)
	require.NotEmpty(t, out)
	t.Logf("live draft: %s", out)
}
```

- [ ] **Step 2: Verify the live test is excluded from the normal suite**

Run: `go test ./internal/qwen/ -v`
Expected: PASS, and the `Live` tests do NOT appear (excluded by the `live` build tag).
Run: `go vet -tags live ./internal/qwen/`
Expected: clean (compiles under the tag).

- [ ] **Step 3: Wire the Qwen client into `cmd/server/main.go`**

In `cmd/server/main.go`, add `"github.com/lemonishi/supportsentinel/internal/qwen"` to imports, and replace the classifier construction line:
```go
	o := orchestrator.New(s, classify.NewFake(), alert.NewLog(), cfg.ConfidenceThreshold)
```
with:
```go
	var clf domain.Classifier
	if cfg.DashScopeAPIKey != "" {
		clf = qwen.New(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.QwenModel, nil)
		log.Printf("classifier: Qwen via DashScope (model=%s)", cfg.QwenModel)
	} else {
		clf = classify.NewFake()
		log.Printf("classifier: fake (DASHSCOPE_API_KEY not set)")
	}
	o := orchestrator.New(s, clf, alert.NewLog(), cfg.ConfidenceThreshold)
```
Add `"github.com/lemonishi/supportsentinel/internal/domain"` to imports (for `domain.Classifier`). Keep the `classify` import (still used for the fallback).

- [ ] **Step 4: Verify build and full suite**

Run:
```bash
go vet ./...
go build ./...
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5433/supportsentinel_test?sslmode=disable' go test ./...
```
Expected: build clean; all tests pass (qwen contract tests run; live tests excluded; DB tests run).

- [ ] **Step 5: Update `CLAUDE.md`**

Under the "## Stack" AI-core bullet, update to note the client is implemented:
```markdown
- AI core: Qwen via Alibaba Cloud DashScope — IMPLEMENTED in `internal/qwen/client.go`
  (the primary "Proof of Alibaba Cloud" artifact). OpenAI-compatible endpoint,
  JSON-mode classification with validation + one re-prompt, free-text drafts,
  bounded retry. `cmd/server` uses it when `DASHSCOPE_API_KEY` is set, else the
  fake classifier. Live smoke test: `go test -tags live ./internal/qwen/`.
```

- [ ] **Step 6: Commit**

```bash
git add internal/qwen/live_test.go cmd/server/main.go CLAUDE.md
git commit -m "feat(qwen): live smoke test and wire Qwen into the server"
```

---

## Plan 2 Definition of Done

- [ ] `go build ./...` and `go vet ./...` clean.
- [ ] `go test ./...` passes (qwen contract tests via httptest; live tests excluded; DB tests pass with `TEST_DATABASE_URL`).
- [ ] `internal/qwen/client.go` clearly calls the Alibaba Cloud DashScope endpoint with a Bearer API key (the Proof of Alibaba Cloud file).
- [ ] With `DASHSCOPE_API_KEY` set, `make dev` logs "classifier: Qwen via DashScope" and a real email POST gets a Qwen classification; without the key it logs the fake-classifier fallback.
- [ ] Manual: `DASHSCOPE_API_KEY=… go test -tags live ./internal/qwen/ -run Live -v` returns plausible live classifications/drafts.

---

## Roadmap — Subsequent Plans (unchanged)

- **Plan 3 — Tool layer** (`lookup_customer`, `lookup_similar_tickets`) as DashScope function-calling tools during Classify; record into `classifications.tools_used`.
- **Plan 4 — React dashboard** (queue, detail, both checkpoint controls, audit timeline; `//go:embed`).
- **Plan 5 — IMAP ingestion + Slack/email alerting.**
- **Plan 6 — Eval harness + gold dataset + threshold calibration.**
- **Plan 7 — Deployment + submission deliverables.**
