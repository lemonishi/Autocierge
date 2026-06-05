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

// Classify is implemented in Task 3.
func (c *Client) Classify(ctx context.Context, e domain.Email) (domain.Classification, error) {
	return domain.Classification{}, errors.New("not implemented")
}

// DraftReply is implemented in Task 4.
func (c *Client) DraftReply(ctx context.Context, t domain.Ticket, e domain.Email) (string, error) {
	return "", errors.New("not implemented")
}
