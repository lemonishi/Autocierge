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
	"strings"
	"time"

	"github.com/lemonishi/supportsentinel/internal/domain"
)

const (
	// DefaultBaseURL is the Alibaba Cloud Model Studio (DashScope) International
	// (Singapore) OpenAI-compatible endpoint.
	DefaultBaseURL = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	// DefaultModel is the default Qwen model for classification.
	DefaultModel = "qwen-max"

	maxAttempts      = 3
	maxToolIterations = 5
)

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

// Client is a Qwen classifier backed by Alibaba Cloud DashScope.
type Client struct {
	apiKey       string
	baseURL      string
	model        string
	httpClient   *http.Client
	retryBackoff time.Duration // initial backoff; doubles each retry
	tools        ToolBox
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
		httpClient:   httpClient,
		retryBackoff: 500 * time.Millisecond,
	}
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

const classifySystemPrompt = `You are a support-ticket classifier. Classify the customer's email and respond with ONLY a JSON object (no prose, no code fences) with exactly these fields:
{"urgency": one of ["low","normal","high","critical"],
 "type": one of ["billing","technical","account","feature_request","general"],
 "department": one of ["billing","engineering","accounts","product","support_tier1"],
 "confidence": a number between 0 and 1 (your confidence in the classification),
 "reasoning": a one-sentence explanation}
Use "critical" only for outages, data loss, or urgent business impact.`

const reclassifyPrompt = `Your previous response was not valid JSON in the required schema. Respond again with ONLY the JSON object described, nothing else.`

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
	s = s[3:] // drop the opening ```
	// Drop an optional language tag on the first line (e.g. "json"/"JSON").
	if nl := strings.IndexByte(s, '\n'); nl != -1 {
		firstLine := strings.TrimSpace(s[:nl])
		if firstLine == "" || !strings.ContainsAny(firstLine, "{[") {
			s = s[nl+1:]
		}
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

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
