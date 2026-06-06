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
