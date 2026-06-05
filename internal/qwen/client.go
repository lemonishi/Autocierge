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

	maxAttempts = 3
)

// Client is a Qwen classifier backed by Alibaba Cloud DashScope.
type Client struct {
	apiKey       string
	baseURL      string
	model        string
	httpClient   *http.Client
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
		httpClient:   httpClient,
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
			return "", fmt.Errorf("build dashscope request: %w", err)
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
					return "", fmt.Errorf("dashscope request cancelled: %w", ctx.Err())
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
					return "", fmt.Errorf("dashscope request cancelled: %w", ctx.Err())
				}
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

// DraftReply is implemented in Task 4.
func (c *Client) DraftReply(ctx context.Context, t domain.Ticket, e domain.Email) (string, error) {
	return "", errors.New("not implemented")
}
